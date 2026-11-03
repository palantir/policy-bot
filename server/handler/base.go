// Copyright 2018 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"context"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	LogKeyGitHubSHA = "github_sha"
)

type Base struct {
	githubapp.ClientCreator

	Installations githubapp.InstallationsService
	GlobalCache   pull.GlobalCache
	ConfigFetcher *ConfigFetcher
	BaseConfig    *baseapp.HTTPConfig
	PullOpts      *PullEvaluationOptions
	Coalescer     *Coalescer

	AppName string
}

// postStatusDelays are the waits before each retry of a status that failed to post on a transient error
var postStatusDelays = []time.Duration{time.Second, 3 * time.Second, 9 * time.Second}

// PostStatus posts a GitHub commit status with consistent logging.
func PostStatus(ctx context.Context, client *github.Client, owner, repo, ref string, status github.RepoStatus) error {
	zerolog.Ctx(ctx).Info().Msgf("Setting %q status on %s to %s: %s", status.GetContext(), ref, status.GetState(), status.GetDescription())
	_, _, err := client.Repositories.CreateStatus(ctx, owner, repo, ref, status)
	for _, delay := range postStatusDelays {
		if !IsTransient(err) {
			break
		}
		zerolog.Ctx(ctx).Warn().Err(err).Msgf("Retrying %q status on %s in %s", status.GetContext(), ref, delay)
		select {
		case <-ctx.Done():
			return errors.WithStack(err)
		case <-time.After(delay):
		}
		_, _, err = client.Repositories.CreateStatus(ctx, owner, repo, ref, status)
	}
	return errors.WithStack(err)
}

func (b *Base) PreparePRContext(ctx context.Context, installationID int64, pr *github.PullRequest) (context.Context, zerolog.Logger) {
	ctx, logger := githubapp.PreparePRContext(ctx, installationID, pr.GetBase().GetRepo(), pr.GetNumber())

	logger = logger.With().Str(LogKeyGitHubSHA, pr.GetHead().GetSHA()).Logger()
	ctx = logger.WithContext(ctx)

	return ctx, logger
}

func (b *Base) NewEvalContext(ctx context.Context, installationID int64, loc pull.Locator) (*EvalContext, error) {
	client, err := b.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	v4client, err := b.NewInstallationV4Client(installationID)
	if err != nil {
		return nil, err
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, loc.Owner, b.Installations, b.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, b.GlobalCache, client, v4client, loc)
	if err != nil {
		return nil, err
	}

	baseBranch, headBranch := prctx.Branches()
	owner := prctx.RepositoryOwner()
	repository := prctx.RepositoryName()

	fetchedConfig := b.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repository, b.PullOpts.PolicyBranch(baseBranch, headBranch))

	return &EvalContext{
		Client:   client,
		V4Client: v4client,

		Options:   b.PullOpts,
		PublicURL: b.BaseConfig.PublicURL,

		PullContext: prctx,
		Config:      fetchedConfig,
	}, nil
}

// EvaluateCoalesced evaluates a pull request for an event, held behind a recent evaluation of it when there is a Coalescer.
// runs are workflow runs the event reported, which the runs API may not list yet.
func (b *Base) EvaluateCoalesced(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator, runs ...*github.WorkflowRun) error {
	e := &evaluation{installationID: installationID, trigger: trigger, loc: loc, runs: runs, run: b.evaluate, events: 1}
	if b.Coalescer == nil {
		return b.evaluate(ctx, e)
	}
	return b.Coalescer.Evaluate(ctx, e)
}

func (b *Base) Evaluate(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator) error {
	return b.evaluate(ctx, &evaluation{installationID: installationID, trigger: trigger, loc: loc})
}

func (b *Base) evaluate(ctx context.Context, e *evaluation) error {
	evalCtx, err := b.NewEvalContext(ctx, e.installationID, e.loc)
	if err != nil {
		return errors.Wrap(err, "failed to create evaluation context")
	}
	if ghc, ok := evalCtx.PullContext.(*pull.GitHubContext); ok {
		for _, run := range e.runs {
			ghc.AddWorkflowRun(run)
		}
	}
	return evalCtx.Evaluate(ctx, e.trigger)
}

// ignoresStatus reports whether the policy on baseRef, or the default branch when empty, reads no status or check named name.
// A config that is missing, unreadable or invalid is never ignored, so the evaluation still runs and reports it.
func (b *Base) ignoresStatus(ctx context.Context, client *github.Client, owner, repo, baseRef, name string) bool {
	if name == "" {
		return false
	}

	fetched := b.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repo, baseRef)
	if fetched.Config == nil || fetched.LoadError != nil || fetched.ParseError != nil {
		return false
	}

	_, ok := fetched.Config.ReferencedStatuses()[name]
	return !ok
}
