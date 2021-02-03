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

	"github.com/google/go-github/v32/github"
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

	LogKeyGitHubSHA = "github_sha"
)

type Base struct {
	githubapp.ClientCreator

	Installations githubapp.InstallationsService
	ConfigFetcher *ConfigFetcher
	BaseConfig    *baseapp.HTTPConfig
	PullOpts      *PullEvaluationOptions

	AppName string
}

type PullEvaluationOptions struct {
	PolicyPath string `yaml:"policy_path"`

	// StatusCheckContext will be used to create the status context. It will be used in the following
	// pattern: <StatusCheckContext>: <Base Branch Name>
	StatusCheckContext string `yaml:"status_check_context"`

	// PostInsecureStatusChecks enables the sending of a second status using just StatusCheckContext as the context,
	// no templating. This is turned off by default. This is to support legacy workflows that depend on the original
	// context behaviour, and will be removed in 2.0
	PostInsecureStatusChecks bool `yaml:"post_insecure_status_checks"`

	// This field is unused but is left to avoid breaking configuration files:
	// yaml.UnmarshalStrict returns an error for unmapped fields
	//
	// TODO(bkeyes): remove in version 2.0
	Deprecated_AppName string `yaml:"app_name"`
}

func (p *PullEvaluationOptions) FillDefaults() {
	if p.PolicyPath == "" {
		p.PolicyPath = DefaultPolicyPath
	}

	if p.StatusCheckContext == "" {
		p.StatusCheckContext = DefaultStatusCheckContext
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

func (b *Base) Evaluate(ctx context.Context, installationID int64, trigger common.Trigger, loc pull.Locator) error {
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

	return b.EvaluateFetchedConfig(ctx, prctx, client, fetchedConfig, trigger)
}

func (b *Base) EvaluateFetchedConfig(ctx context.Context, prctx pull.Context, client *github.Client, fetchedConfig FetchedConfig, trigger common.Trigger) error {
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

	policyTrigger := evaluator.Trigger()
	if !trigger.Matches(policyTrigger) {
		logger.Debug().
			Str("event_trigger", trigger.String()).
			Str("policy_trigger", policyTrigger.String()).
			Msg("No evaluation necessary for this trigger, skipping")
		return nil
	}

	result := evaluator.Evaluate(ctx, prctx)
	if result.Error != nil {
		statusMessage := fmt.Sprintf("Error evaluating policy defined by %s. This may be temporary. Create a PR comment to try again. ", fetchedConfig)
		logger.Warn().Err(result.Error).Msg(statusMessage)
		err := b.PostStatus(ctx, prctx, client, "error", statusMessage)
		return err
	}

	statusDescription := result.StatusDescription
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

	if err := b.PostStatus(ctx, prctx, client, statusState, statusDescription); err != nil {
		return err
	}

	if statusState == "pending" && !prctx.IsDraft() {
		if reqs := reviewer.FindRequests(&result); len(reqs) > 0 {
			logger.Debug().Msgf("Found %d pending rules with review requests enabled", len(reqs))
			return b.requestReviews(ctx, prctx, client, reqs)
		}
		logger.Debug().Msgf("No pending rules have review requests enabled, skipping reviewer assignment")
	}

	return nil
}

func (b *Base) requestReviews(ctx context.Context, prctx pull.Context, client *github.Client, reqs []*common.Result) error {
	const maxDelayMillis = 2500

	logger := zerolog.Ctx(ctx)

	// Seed the random source with the PR creation time so that repeated
	// evaluations produce the same set of reviewers. This is required to avoid
	// duplicate requests on later evaluations.
	r := rand.New(rand.NewSource(prctx.CreatedAt().UnixNano()))
	selection, err := reviewer.SelectReviewers(ctx, prctx, reqs, r)
	if err != nil {
		return errors.Wrap(err, "failed to select reviewers")
	}

	if selection.IsEmpty() {
		logger.Debug().Msg("No eligible users or teams found for review")
		return nil
	}

	// This is a terrible strategy to avoid conflicts between closely spaced
	// events assigning the same reviewers, but I expect it to work alright in
	// practice and it avoids any kind of coordination backend:
	//
	// Wait a random amount of time to space out events then check for existing
	// reviewers and apply the difference. The idea is to order two competing
	// events such that one observes the applied reviewers of the other.
	//
	// Use the global random source instead of the per-PR source so that two
	// events for the same PR don't wait for the same amount of time.
	delay := time.Duration(rand.Intn(maxDelayMillis)) * time.Millisecond
	logger.Debug().Msgf("Waiting for %s to spread out reviewer processing", delay)
	time.Sleep(delay)

	// check again if someone assigned a reviewer while we were calculating users to request
	reviewers, err := prctx.RequestedReviewers()
	if err != nil {
		return err
	}

	if diff := selection.Difference(reviewers); !diff.IsEmpty() {
		req := selectionToReviewersRequest(diff)
		logger.Debug().
			Strs("users", req.Reviewers).
			Strs("teams", req.TeamReviewers).
			Msgf("Requesting reviews from %d users and %d teams", len(req.Reviewers), len(req.TeamReviewers))

		_, _, err = client.PullRequests.RequestReviewers(ctx, prctx.RepositoryOwner(), prctx.RepositoryName(), prctx.Number(), req)
		return errors.Wrap(err, "failed to request reviewers")
	}

	logger.Debug().Msg("All selected reviewers are already assigned or were explicitly removed")
	return nil
}

func selectionToReviewersRequest(s reviewer.Selection) github.ReviewersRequest {
	req := github.ReviewersRequest{}

	if len(s.Users) > 0 {
		req.Reviewers = s.Users
	} else {
		req.Reviewers = []string{}
	}

	if len(s.Teams) > 0 {
		req.TeamReviewers = s.Teams
	} else {
		req.TeamReviewers = []string{}
	}

	return req
}
