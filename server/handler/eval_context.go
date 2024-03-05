// Copyright 2022 Palantir Technologies, Inc.
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
	"strings"

	"github.com/google/go-github/v59/github"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
)

// EvalContext contains common fields and methods used to evaluate policy
// requests. Handlers construct an EvalContext once they decide to handle a
// request or event, then call the appropriate methods for each stage of
// evaluation. Handlers with no special requirements can simply call Evaluate.
type EvalContext struct {
	Client   *github.Client
	V4Client *githubv4.Client

	Options   *PullEvaluationOptions
	PublicURL string

	PullContext pull.Context
	Config      FetchedConfig

	// If true, store statuses in the Status field instead of posting them to
	// GitHub. Only the last status is saved, so when this option is enabled,
	// callers should check for a non-nil status after each method call.
	SkipPostStatus bool
	Status         *github.RepoStatus
}

// Evaluate runs the full process for evaluating a pull request.
func (ec *EvalContext) Evaluate(ctx context.Context, trigger common.Trigger) error {
	evaluator, err := ec.ParseConfig(ctx, trigger)
	if err != nil {
		return err
	}
	if evaluator == nil {
		return nil
	}

	result, err := ec.EvaluatePolicy(ctx, evaluator)
	if err != nil {
		return err
	}

	ec.RunPostEvaluateActions(ctx, result, trigger)
	return nil
}

// ParseConfig checks and validates the configuration in the EvalContext and
// returns a non-nil Evaluator if the policy exists, is valid, and requires
// evaluation for the trigger.
func (ec *EvalContext) ParseConfig(ctx context.Context, trigger common.Trigger) (common.Evaluator, error) {
	logger := zerolog.Ctx(ctx)

	fc := ec.Config
	switch {
	case fc.LoadError != nil:
		msg := fmt.Sprintf("Error loading policy from %s", fc.Source)
		logger.Warn().Err(fc.LoadError).Msg(msg)

		ec.PostStatus(ctx, "error", msg)
		return nil, errors.Wrapf(fc.LoadError, "failed to load policy: %s: %s", fc.Source, fc.Path)

	case fc.ParseError != nil:
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(fc.ParseError).Msg(msg)

		ec.PostStatus(ctx, "error", msg)
		return nil, errors.Wrapf(fc.ParseError, "failed to parse policy: %s: %s", fc.Source, fc.Path)

	case fc.Config == nil:
		logger.Debug().Msg("No policy defined for repository")
		return nil, nil
	}

	evaluator, err := policy.ParsePolicy(fc.Config)
	if err != nil {
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(err).Msg(msg)

		ec.PostStatus(ctx, "error", msg)
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

// EvaluatePolicy evaluates the policy for a PR and generates a result. The
// evaluator must be non-nil, meaning callers should check the output of
// ParseConfig before calling this method.
func (ec *EvalContext) EvaluatePolicy(ctx context.Context, evaluator common.Evaluator) (common.Result, error) {
	logger := zerolog.Ctx(ctx)

	result := evaluator.Evaluate(ctx, ec.PullContext)
	if result.Error != nil {
		msg := fmt.Sprintf("Error evaluating policy in %s: %s", ec.Config.Source, ec.Config.Path)
		logger.Warn().Err(result.Error).Msg(msg)

		ec.PostStatus(ctx, "error", msg)
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

	ec.PostStatus(ctx, statusState, statusDescription)
	return result, nil
}

// RunPostEvaluateActions executes additional actions that should happen after
// evaluation completes, like assigning reviewers or dismissing reviews. These
// actions happen after a status is posted to GitHub for the main evaluation.
//
// Post-evaluate actions are best effort, so this function logs failures
// instead of returning an error.
func (ec *EvalContext) RunPostEvaluateActions(ctx context.Context, result common.Result, trigger common.Trigger) {
	logger := zerolog.Ctx(ctx)

	if err := ec.requestReviewsForResult(ctx, trigger, result); err != nil {
		logger.Error().Err(err).Msg("Failed to request reviewers")
	}

	if err := ec.dismissStaleReviewsForResult(ctx, result); err != nil {
		logger.Error().Err(err).Msg("Failed to dismiss stale reviews")
	}
}

// PostStatus posts a status for the evaluated PR.
func (ec *EvalContext) PostStatus(ctx context.Context, state, message string) {
	logger := zerolog.Ctx(ctx)

	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	sha := ec.PullContext.HeadSHA()
	base, _ := ec.PullContext.Branches()

	publicURL := strings.TrimSuffix(ec.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, ec.PullContext.Number())

	status := github.RepoStatus{
		State:       &state,
		Context:     github.String(fmt.Sprintf("%s: %s", ec.Options.StatusCheckContext, base)),
		Description: &message,
		TargetURL:   &detailsURL,
	}

	if ec.SkipPostStatus {
		ec.Status = &status
		return
	}

	if !ec.PullContext.IsOpen() {
		logger.Info().Msg("Skipping status update because PR state is not open")
		return
	}

	if err := PostStatus(ctx, ec.Client, owner, repo, sha, &status); err != nil {
		logger.Err(err).Msg("Failed to post repo status")
	}
	if ec.Options.PostInsecureStatusChecks {
		status.Context = github.String(ec.Options.StatusCheckContext)
		if err := PostStatus(ctx, ec.Client, owner, repo, sha, &status); err != nil {
			logger.Err(err).Msg("Failed to post insecure repo status")
		}
	}
}
