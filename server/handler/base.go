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
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/reviewer"
	"github.com/palantir/policy-bot/pull"
)

const (
	DefaultPolicyPath         = ".policy.yml"
	DefaultStatusCheckContext = "policy-bot"
	DefaultAppName            = "policy-bot"

	LogKeyGitHubSHA = "github_sha"
)

type Base struct {
	githubapp.ClientCreator

	Installations githubapp.InstallationsService
	PullOpts      *PullEvaluationOptions
	ConfigFetcher *ConfigFetcher
	BaseConfig    *baseapp.HTTPConfig
}

type PullEvaluationOptions struct {
	AppName    string `yaml:"app_name"`
	PolicyPath string `yaml:"policy_path"`

	// StatusCheckContext will be used to create the status context. It will be used in the following
	// pattern: <StatusCheckContext>: <Base Branch Name>
	StatusCheckContext string `yaml:"status_check_context"`

	// PostInsecureStatusChecks enables the sending of a second status using just StatusCheckContext as the context,
	// no templating. This is turned off by default. This is to support legacy workflows that depend on the original
	// context behaviour, and will be removed in 2.0
	PostInsecureStatusChecks bool `yaml:"post_insecure_status_checks"`
}

func (p *PullEvaluationOptions) FillDefaults() {
	if p.PolicyPath == "" {
		p.PolicyPath = DefaultPolicyPath
	}

	if p.StatusCheckContext == "" {
		p.StatusCheckContext = DefaultStatusCheckContext
	}

	if p.AppName == "" {
		p.AppName = DefaultAppName
	}
}

func (b *Base) PostStatus(ctx context.Context, prctx pull.Context, client *github.Client, state, message string) error {
	owner := prctx.RepositoryOwner()
	repo := prctx.RepositoryName()
	sha := prctx.HeadSHA()
	base, _ := prctx.Branches()

	publicURL := strings.TrimSuffix(b.BaseConfig.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, prctx.Number())

	contextWithBranch := fmt.Sprintf("%s: %s", b.PullOpts.StatusCheckContext, base)
	status := &github.RepoStatus{
		Context:     &contextWithBranch,
		State:       &state,
		Description: &message,
		TargetURL:   &detailsURL,
	}

	if err := b.postGitHubRepoStatus(ctx, client, owner, repo, sha, status); err != nil {
		return err
	}

	if b.PullOpts.PostInsecureStatusChecks {
		status.Context = &b.PullOpts.StatusCheckContext
		if err := b.postGitHubRepoStatus(ctx, client, owner, repo, sha, status); err != nil {
			return err
		}
	}

	return nil
}

func (b *Base) postGitHubRepoStatus(ctx context.Context, client *github.Client, owner, repo, ref string, status *github.RepoStatus) error {
	logger := zerolog.Ctx(ctx)
	logger.Info().Msgf("Setting %q status on %s to %s: %s", status.GetContext(), ref, status.GetState(), status.GetDescription())
	_, _, err := client.Repositories.CreateStatus(ctx, owner, repo, ref, status)
	return err
}

func (b *Base) PreparePRContext(ctx context.Context, installationID int64, pr *github.PullRequest) (context.Context, zerolog.Logger) {
	ctx, logger := githubapp.PreparePRContext(ctx, installationID, pr.GetBase().GetRepo(), pr.GetNumber())

	logger = logger.With().Str(LogKeyGitHubSHA, pr.GetHead().GetSHA()).Logger()
	ctx = logger.WithContext(ctx)

	return ctx, logger
}

func (b *Base) Evaluate(ctx context.Context, installationID int64, performActions bool, loc pull.Locator) error {
	client, err := b.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	v4client, err := b.NewInstallationV4Client(installationID)
	if err != nil {
		return err
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, loc.Owner, b.Installations, b.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, loc)
	if err != nil {
		return err
	}

	fetchedConfig, err := b.ConfigFetcher.ConfigForPR(ctx, prctx, client)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("failed to fetch policy: %s", fetchedConfig))
	}

	return b.EvaluateFetchedConfig(ctx, prctx, performActions, client, fetchedConfig)
}

func (b *Base) EvaluateFetchedConfig(ctx context.Context, prctx pull.Context, performActions bool, client *github.Client, fetchedConfig FetchedConfig) error {
	logger := zerolog.Ctx(ctx)

	if fetchedConfig.Missing() {
		logger.Debug().Msgf("policy does not exist: %s", fetchedConfig)
		return nil
	}

	if fetchedConfig.Invalid() {
		logger.Warn().Err(fetchedConfig.Error).Msgf("invalid policy: %s", fetchedConfig)
		err := b.PostStatus(ctx, prctx, client, "error", fetchedConfig.Description())
		return err
	}

	evaluator, err := policy.ParsePolicy(fetchedConfig.Config)
	if err != nil {
		statusMessage := fmt.Sprintf("Invalid policy defined by %s", fetchedConfig)
		logger.Debug().Err(err).Msg(statusMessage)
		err := b.PostStatus(ctx, prctx, client, "error", statusMessage)
		return err
	}

	result := evaluator.Evaluate(ctx, prctx)
	if result.Error != nil {
		statusMessage := fmt.Sprintf("Error evaluating policy defined by %s", fetchedConfig)
		logger.Warn().Err(result.Error).Msg(statusMessage)
		err := b.PostStatus(ctx, prctx, client, "error", statusMessage)
		return err
	}

	statusDescription := result.Description
	var statusState string
	switch result.Status {
	case common.StatusApproved:
		statusState = "success"
	case common.StatusDisapproved:
		statusState = "failure"
	case common.StatusPending:
		statusState = "pending"
	case common.StatusSkipped:
		statusState = "error"
		statusDescription = "All rules were skipped. At least one rule must match."
	default:
		return errors.Errorf("evaluation resulted in unexpected state: %s", result.Status)
	}

	err = b.PostStatus(ctx, prctx, client, statusState, statusDescription)
	if err != nil {
		return err
	}

	if performActions && statusState == "pending" && !prctx.IsDraft() {
		hasReviewers, err := prctx.HasReveiwers()
		if err != nil {
			logger.Warn().Err(err).Msg("Unable to list request reviewers")
		}

		if !hasReviewers {
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			requestedUsers, err := reviewer.FindRandomRequesters(ctx, prctx, result, r)
			if err != nil {
				logger.Warn().Err(err).Msg("Unable to select random request reviewers")
			}

			if len(requestedUsers) > 0 {
				reviewers := github.ReviewersRequest{
					Reviewers:     requestedUsers,
					TeamReviewers: []string{},
				}

				logger.Debug().Msgf("PR is not in draft, there are no current reviewers, and reviews are requested from %q users", requestedUsers)
				_, _, err = client.PullRequests.RequestReviewers(ctx, prctx.RepositoryOwner(), prctx.RepositoryName(), prctx.Number(), reviewers)
				if err != nil {
					logger.Warn().Err(err).Msg("Unable to request reviewers")
				}
			} else {
				logger.Debug().Msg("No users found for review, or no users were requested")
			}
		} else {
			logger.Debug().Msg("PR has existing reviewers, not adding anyone")
		}
	}

	return nil
}
