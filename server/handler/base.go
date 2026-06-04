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

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
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

	AppName string
	AppID   int64

	Debouncer         *StatusDebouncer
	RateLimitDeferrer *RateLimitDeferrer
}

// PostCheckRun creates a GitHub check run with consistent logging.
func PostCheckRun(ctx context.Context, client *github.Client, owner, repo string, opts github.CreateCheckRunOptions) error {
	conclusion := "in_progress"
	if opts.Conclusion != nil {
		conclusion = *opts.Conclusion
	}
	description := ""
	if opts.Output != nil {
		description = opts.Output.GetTitle()
	}
	zerolog.Ctx(ctx).Info().Msgf("Setting %q check run on %s to %s: %s", opts.Name, opts.HeadSHA, conclusion, description)
	_, _, err := client.Checks.CreateCheckRun(ctx, owner, repo, opts)
	return errors.WithStack(err)
}

// NewCheckRunOptions builds a CreateCheckRunOptions from the legacy status
// state values (pending, success, failure, error) used throughout the codebase.
func NewCheckRunOptions(name, headSHA, state, message string, detailsURL *string) github.CreateCheckRunOptions {
	now := &github.Timestamp{Time: time.Now()}
	opts := github.CreateCheckRunOptions{
		Name:       name,
		HeadSHA:    headSHA,
		DetailsURL: detailsURL,
		StartedAt:  now,
		Output: &github.CheckRunOutput{
			Title:   &message,
			Summary: &message,
		},
	}

	switch state {
	case "pending":
		opts.Status = github.Ptr("in_progress")
	case "success":
		opts.Status = github.Ptr("completed")
		opts.Conclusion = github.Ptr("success")
		opts.CompletedAt = now
	case "failure":
		opts.Status = github.Ptr("completed")
		opts.Conclusion = github.Ptr("failure")
		opts.CompletedAt = now
	case "error":
		opts.Status = github.Ptr("completed")
		opts.Conclusion = github.Ptr("action_required")
		opts.CompletedAt = now
	}

	return opts
}

func (b *Base) PreparePRContext(ctx context.Context, installationID int64, pr *github.PullRequest) (context.Context, zerolog.Logger) {
	ctx, logger := githubapp.PreparePRContext(ctx, installationID, pr.GetBase().GetRepo(), pr.GetNumber())

	logger = logger.With().Str(LogKeyGitHubSHA, pr.GetHead().GetSHA()).Logger()
	ctx = logger.WithContext(ctx)

	return ctx, logger
}

func (b *Base) NewEvalContext(ctx context.Context, installationID int64, loc pull.Locator) (*EvalContext, error) {
	return b.newEvalContext(ctx, installationID, loc, nil)
}

func (b *Base) newEvalContextWithConfig(ctx context.Context, installationID int64, loc pull.Locator, fetchedConfig FetchedConfig) (*EvalContext, error) {
	return b.newEvalContext(ctx, installationID, loc, &fetchedConfig)
}

func (b *Base) newEvalContext(ctx context.Context, installationID int64, loc pull.Locator, fetchedConfig *FetchedConfig) (*EvalContext, error) {
	client, err := b.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	v4client, err := b.NewInstallationV4Client(installationID)
	if err != nil {
		return nil, err
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, loc.Owner, b.Installations, b.ClientCreator, b.GlobalCache)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, b.GlobalCache, client, v4client, loc)
	if err != nil {
		return nil, err
	}

	baseBranch, _ := prctx.Branches()
	owner := prctx.RepositoryOwner()
	repository := prctx.RepositoryName()

	if fetchedConfig == nil {
		fetchedConfig = github.Ptr(b.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repository, baseBranch))
	}

	return &EvalContext{
		Client:   client,
		V4Client: v4client,

		Options:   b.PullOpts,
		PublicURL: b.BaseConfig.PublicURL,

		PullContext: prctx,
		Config:      *fetchedConfig,
	}, nil
}

func (b *Base) Evaluate(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator) error {
	key := DebounceKey(loc.Owner, loc.Repo, loc.Number)
	if b.Debouncer != nil {
		trailingFn := func(eventCtx context.Context, accumulated common.Trigger, coalesced int) {
			tctx, span := StartDebounceTrailingSpan(eventCtx)
			defer span.End()
			span.SetAttributes(RepoAttrs(loc.Owner, loc.Repo)...)
			span.SetAttributes(
				attribute.Int(AttrPRNumber, loc.Number),
				attribute.String(AttrPolicyTrigger, accumulated.String()),
				attribute.String(AttrDebounceKey, key),
				attribute.String(AttrDebounceAccumulatedTrigger, accumulated.String()),
				attribute.Int(AttrDebounceCoalesced, coalesced),
			)

			logger := zerolog.Ctx(tctx)
			logger.Debug().Msgf("Running trailing evaluation for %s/%s#%d (accumulated trigger: %s, coalesced: %d)", loc.Owner, loc.Repo, loc.Number, accumulated, coalesced)
			if err := b.evaluateOnce(tctx, installationID, accumulated, loc, key); err != nil {
				RecordError(span, &err)
				logger.Error().Err(err).Msgf("Trailing evaluation failed for %s/%s#%d", loc.Owner, loc.Repo, loc.Number)
			}
		}
		if !b.Debouncer.Deduplicate(ctx, key, trigger, trailingFn) {
			zerolog.Ctx(ctx).Debug().Msgf("Debounced evaluation for %s/%s#%d (trigger: %s)", loc.Owner, loc.Repo, loc.Number, trigger)
			return nil
		}
	}
	return b.evaluateOnce(ctx, installationID, trigger, loc, key)
}

func (b *Base) doEvaluate(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator) error {
	evalCtx, err := b.NewEvalContext(ctx, installationID, loc)
	if err != nil {
		return errors.Wrap(err, "failed to create evaluation context")
	}
	return evalCtx.Evaluate(ctx, trigger)
}

func (b *Base) evaluateOnce(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator, key string) error {
	if b.RateLimitDeferrer != nil && b.RateLimitDeferrer.DeferIfActive(ctx, installationID, key, trigger, func(deferredCtx context.Context, deferredTrigger common.Trigger) {
		if err := b.evaluateOnce(deferredCtx, installationID, deferredTrigger, loc, key); err != nil {
			zerolog.Ctx(deferredCtx).Error().Err(err).Msgf("Deferred evaluation failed for %s/%s#%d", loc.Owner, loc.Repo, loc.Number)
		}
	}) {
		zerolog.Ctx(ctx).Warn().Msgf("Deferring evaluation for %s/%s#%d because installation %d is rate limited", loc.Owner, loc.Repo, loc.Number, installationID)
		return nil
	}

	if err := b.doEvaluate(ctx, installationID, trigger, loc); err != nil {
		if resetAt, ok := rateLimitResetTime(err); ok && b.RateLimitDeferrer != nil {
			b.RateLimitDeferrer.DeferUntil(ctx, installationID, key, trigger, resetAt, func(deferredCtx context.Context, deferredTrigger common.Trigger) {
				if evalErr := b.evaluateOnce(deferredCtx, installationID, deferredTrigger, loc, key); evalErr != nil {
					zerolog.Ctx(deferredCtx).Error().Err(evalErr).Msgf("Deferred evaluation failed for %s/%s#%d", loc.Owner, loc.Repo, loc.Number)
				}
			})
			zerolog.Ctx(ctx).Warn().Err(err).Time("github_rate_limit_reset", resetAt).Msgf("Rate limited evaluating %s/%s#%d; deferred retry scheduled", loc.Owner, loc.Repo, loc.Number)
			return nil
		}
		return err
	}
	return nil
}
