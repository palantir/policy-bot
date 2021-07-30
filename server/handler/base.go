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

	"github.com/google/go-github/v37/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/reviewer"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	DefaultPolicyPath       = ".policy.yml"
	DefaultSharedRepository = ".github"
	DefaultSharedPolicyPath = "policy.yml"

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

	SharedRepository string `yaml:"shared_repository"`
	SharedPolicyPath string `yaml:"shared_policy_path"`

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
	if p.SharedRepository == "" {
		p.SharedRepository = DefaultSharedRepository
	}
	if p.SharedPolicyPath == "" {
		p.SharedPolicyPath = DefaultSharedPolicyPath
	}

	if p.StatusCheckContext == "" {
		p.StatusCheckContext = DefaultStatusCheckContext
	}
}

func (b *Base) PostStatus(ctx context.Context, prctx pull.Context, client *github.Client, state, message string) {
	logger := zerolog.Ctx(ctx)

	owner := prctx.RepositoryOwner()
	repo := prctx.RepositoryName()
	sha := prctx.HeadSHA()
	base, _ := prctx.Branches()

	if !prctx.IsOpen() {
		logger.Info().Msg("Skipping status update because PR state is not open")
		return
	}

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
		logger.Err(errors.WithStack(err)).Msg("Failed to post repo status")
		return
	}

	if b.PullOpts.PostInsecureStatusChecks {
		status.Context = &b.PullOpts.StatusCheckContext
		if err := b.postGitHubRepoStatus(ctx, client, owner, repo, sha, status); err != nil {
			logger.Err(errors.WithStack(err)).Msg("Failed to post repo status with StatusCheckContext")
		}
	}

	return
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

	fetchedConfig := b.ConfigFetcher.ConfigForPR(ctx, prctx, client)
	evaluator, err := b.ValidateFetchedConfig(ctx, prctx, client, fetchedConfig, trigger)
	if err != nil {
		return err
	}
	if evaluator == nil {
		return nil
	}

	result, err := b.EvaluateFetchedConfig(ctx, prctx, client, evaluator, fetchedConfig)
	if err != nil {
		return err
	}

	return b.RequestReviewsForResult(ctx, prctx, client, trigger, result)
}

func (b *Base) ValidateFetchedConfig(ctx context.Context, prctx pull.Context, client *github.Client, fc FetchedConfig, trigger common.Trigger) (common.Evaluator, error) {
	logger := zerolog.Ctx(ctx)

	switch {
	case fc.LoadError != nil:
		msg := fmt.Sprintf("Error loading policy from %s", fc.Source)
		logger.Warn().Err(fc.LoadError).Msg(msg)

		b.PostStatus(ctx, prctx, client, "error", msg)
		return nil, errors.Wrapf(fc.LoadError, "failed to load policy: %s: %s", fc.Source, fc.Path)

	case fc.ParseError != nil:
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(fc.ParseError).Msg(msg)

		b.PostStatus(ctx, prctx, client, "error", msg)
		return nil, errors.Wrapf(fc.ParseError, "failed to parse policy: %s: %s", fc.Source, fc.Path)

	case fc.Config == nil:
		logger.Debug().Msg("No policy defined for repository")
		return nil, nil
	}

	evaluator, err := policy.ParsePolicy(fc.Config)
	if err != nil {
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(err).Msg(msg)

		b.PostStatus(ctx, prctx, client, "error", msg)
		return nil, errors.Wrapf(err, "failed to create evaluator: %s: %s", fc.Source, fc.Path)
	}

	policyTrigger := evaluator.Trigger()
	if !trigger.Matches(policyTrigger) {
		logger.Debug().
			Str("event_trigger", trigger.String()).
			Str("policy_trigger", policyTrigger.String()).
			Msg("No evaluation necessary for this trigger, skipping")
		return nil, nil
	}

	return evaluator, nil
}

func (b *Base) EvaluateFetchedConfig(ctx context.Context, prctx pull.Context, client *github.Client, evaluator common.Evaluator, fc FetchedConfig) (common.Result, error) {
	logger := zerolog.Ctx(ctx)

	result := evaluator.Evaluate(ctx, prctx)
	if result.Error != nil {
		msg := fmt.Sprintf("Error evaluating policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(result.Error).Msg(msg)

		b.PostStatus(ctx, prctx, client, "error", msg)
		return result, result.Error
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
		err := errors.Errorf("Evaluation resulted in unexpected status: %s", result.Status)
		return result, err
	}

	b.PostStatus(ctx, prctx, client, statusState, statusDescription)

	return result, nil
}

func (b *Base) RequestReviewsForResult(ctx context.Context, prctx pull.Context, client *github.Client, trigger common.Trigger, result common.Result) error {
	logger := zerolog.Ctx(ctx)

	if prctx.IsDraft() || result.Status != common.StatusPending {
		return nil
	}

	// As of 2021-05-19, there are no predicates that use comments or reviews
	// to enable or disable rules. This means these events will never cause a
	// change in reviewer assignment and we can skip the whole process.
	reviewTrigger := ^(common.TriggerComment | common.TriggerReview)
	if !trigger.Matches(reviewTrigger) {
		return nil
	}

	if reqs := reviewer.FindRequests(&result); len(reqs) > 0 {
		logger.Debug().Msgf("Found %d pending rules with review requests enabled", len(reqs))
		return b.requestReviews(ctx, prctx, client, reqs)
	}

	logger.Debug().Msgf("No pending rules have review requests enabled, skipping reviewer assignment")
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

	// Get existing reviewers _after_ computing the selection and sleeping in
	// case something assigned reviewers (or left reviews) in the meantime.
	reviewers, err := prctx.RequestedReviewers()
	if err != nil {
		return err
	}

	// After a user leaves a review, GitHub removes the user from the request
	// list. If the review didn't actually change the state of the requesting
	// rule, policy-bot may request the same user again. To avoid this, include
	// any reviews on the head commit in the set of existing reviewers to avoid
	// re-requesting them until the content changes.
	head := prctx.HeadSHA()
	reviews, err := prctx.Reviews()
	if err != nil {
		return err
	}
	for _, r := range reviews {
		if r.SHA == head {
			reviewers = append(reviewers, &pull.Reviewer{
				Type: pull.ReviewerUser,
				Name: r.Author,
			})

			for _, team := range r.Teams {
				reviewers = append(reviewers, &pull.Reviewer{
					Type: pull.ReviewerTeam,
					Name: team,
				})
			}
		}
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
