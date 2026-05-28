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
	"net/url"
	"strings"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"go.opentelemetry.io/otel/attribute"
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

	// If true, store check run options in the Status field instead of posting
	// them to GitHub. Only the last status is saved, so when this option is
	// enabled, callers should check for a non-nil status after each method call.
	SkipPostStatus bool
	Status         *github.CreateCheckRunOptions
}

// Evaluate runs the full process for evaluating a pull request.
func (ec *EvalContext) Evaluate(ctx context.Context, trigger common.Trigger) (err error) {
	ctx, span := StartChildSpan(ctx, "policy.evaluate")
	defer span.End()
	defer RecordError(span, &err)
	span.SetAttributes(attribute.String(AttrPolicyTrigger, trigger.String()))

	evaluator, err := ec.ParseConfig(ctx, trigger)
	if err != nil {
		return err
	}
	if evaluator == nil {
		span.SetAttributes(attribute.String(AttrPolicySkipReason, SkipReasonNoPolicy))
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
func (ec *EvalContext) ParseConfig(ctx context.Context, trigger common.Trigger) (evaluator common.Evaluator, err error) {
	return parseFetchedConfigWithSpan(ctx, ec.Config, ec.Options, trigger, ec.PostStatus)
}

func parseFetchedConfigWithSpan(ctx context.Context, fc FetchedConfig, opts *PullEvaluationOptions, trigger common.Trigger, postStatus postStatusFunc) (evaluator common.Evaluator, err error) {
	ctx, span := StartChildSpan(ctx, "policy.parse_config")
	defer span.End()
	defer RecordError(span, &err)
	span.SetAttributes(
		attribute.String(AttrPolicyConfigSource, fc.Source),
		attribute.String(AttrPolicyConfigPath, fc.Path),
	)

	return parseFetchedConfig(ctx, fc, opts, trigger, postStatus)
}

type postStatusFunc func(ctx context.Context, state, message string)

func parseFetchedConfig(ctx context.Context, fc FetchedConfig, evalOpts *PullEvaluationOptions, trigger common.Trigger, postStatus postStatusFunc) (common.Evaluator, error) {
	logger := zerolog.Ctx(ctx)

	switch {
	case fc.LoadError != nil:
		msg := fmt.Sprintf("Error loading policy from %s", fc.Source)
		logger.Warn().Err(fc.LoadError).Msg(msg)

		if postStatus != nil {
			postStatus(ctx, "error", msg)
		}
		return nil, errors.Wrapf(fc.LoadError, "failed to load policy: %s: %s", fc.Source, fc.Path)

	case fc.ParseError != nil:
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(fc.ParseError).Msg(msg)

		if postStatus != nil {
			postStatus(ctx, "error", msg)
		}
		return nil, errors.Wrapf(fc.ParseError, "failed to parse policy: %s: %s", fc.Source, fc.Path)

	case fc.Config == nil:
		logger.Debug().Msg("No policy defined for repository")
		return nil, nil
	}

	if evalOpts == nil {
		evalOpts = &PullEvaluationOptions{}
	}
	policyOpts := &policy.GlobalOptions{
		IgnoreEditedComments: evalOpts.IgnoreEditedComments,
		ApprovalDefaults:     evalOpts.ApprovalDefaults,
	}

	evaluator, err := policy.ParsePolicy(fc.Config, policyOpts)
	if err != nil {
		msg := fmt.Sprintf("Invalid policy in %s: %s", fc.Source, fc.Path)
		logger.Warn().Err(err).Msg(msg)

		if postStatus != nil {
			postStatus(ctx, "error", msg)
		}
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
func (ec *EvalContext) EvaluatePolicy(ctx context.Context, evaluator common.Evaluator) (_ common.Result, err error) {
	ctx, span := StartChildSpan(ctx, "policy.evaluate_policy")
	defer span.End()
	defer RecordError(span, &err)

	logger := zerolog.Ctx(ctx)

	ec.prefetchPullData()

	result := evaluator.Evaluate(ctx, ec.PullContext)
	if result.Error != nil {
		msg := fmt.Sprintf("Error evaluating policy in %s: %s", ec.Config.Source, ec.Config.Path)
		logger.Warn().Err(result.Error).Msg(msg)

		if !isTransientClientError(result.Error) {
			ec.PostStatus(ctx, "error", msg)
		}
		return result, result.Error
	}

	span.SetAttributes(attribute.String(AttrPolicyStatus, result.Status.String()))

	statusDescription := result.StatusDescription

	var statusState string
	switch result.Status {
	case common.StatusApproved:
		statusState = "success"
	case common.StatusDisapproved:
		statusState = "failure"
	case common.StatusPending:
		// Policy-level setting takes precedence over server-level
		pendingAsFailure := ec.Options.PendingAsFailure
		if ec.Config.Config != nil && ec.Config.Config.PendingAsFailure != nil {
			pendingAsFailure = *ec.Config.Config.PendingAsFailure
		}
		// When pending_as_failure is enabled but the pending status is only
		// due to conditions (e.g., CI checks still running) and not missing
		// actor approvals, report "pending" instead of "failure". This
		// allows tools like Kodiak to distinguish between "waiting for CI"
		// (should wait) and "needs human action" (should not merge).
		if pendingAsFailure && !result.PendingOnConditionsOnly {
			statusState = "failure"
		} else {
			statusState = "pending"
		}
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

// prefetchPullData starts background fetches for commonly needed pull request
// data. Call this only after deciding that policy evaluation will run.
func (ec *EvalContext) prefetchPullData() {
	if ghctx, ok := ec.PullContext.(*pull.GitHubContext); ok {
		ghctx.Prefetch()
	}
}

// RunPostEvaluateActions executes additional actions that should happen after
// evaluation completes, like assigning reviewers or dismissing reviews. These
// actions happen after a status is posted to GitHub for the main evaluation.
//
// Post-evaluate actions are best effort, so this function logs failures
// instead of returning an error.
func (ec *EvalContext) RunPostEvaluateActions(ctx context.Context, result common.Result, trigger common.Trigger) {
	ctx, span := StartChildSpan(ctx, "policy.post_evaluate_actions")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	if err := ec.requestReviewsForResult(ctx, trigger, result); err != nil {
		logger.Error().Err(err).Msg("Failed to request reviewers")
		span.RecordError(err)
	}

	if err := ec.dismissStaleReviewsForResult(ctx, result); err != nil {
		logger.Error().Err(err).Msg("Failed to dismiss stale reviews")
		span.RecordError(err)
	}
}

// PostStatus posts a check run for the evaluated PR.
func (ec *EvalContext) PostStatus(ctx context.Context, state, message string) {
	ctx, span := StartChildSpan(ctx, "policy.post_status")
	defer span.End()
	span.SetAttributes(
		attribute.String(AttrPolicyStatus, state),
		attribute.String(AttrSHA, ec.PullContext.HeadSHA()),
	)

	logger := zerolog.Ctx(ctx)

	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	sha := ec.PullContext.HeadSHA()
	base, _ := ec.PullContext.Branches()

	publicURL := strings.TrimSuffix(ec.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, ec.PullContext.Number())

	name := fmt.Sprintf("%s: %s", ec.Options.StatusCheckContext, base)
	opts := NewCheckRunOptions(name, sha, state, message, &detailsURL)

	if ec.SkipPostStatus {
		ec.Status = &opts
		return
	}

	if !ec.PullContext.IsOpen() {
		logger.Info().Msg("Skipping status update because PR state is not open")
		return
	}

	if err := PostCheckRun(ctx, ec.Client, owner, repo, opts); err != nil {
		logger.Err(err).Msg("Failed to post check run")
	}
	if ec.Options.PostInsecureStatusChecks {
		insecureOpts := NewCheckRunOptions(ec.Options.StatusCheckContext, sha, state, message, &detailsURL)
		if err := PostCheckRun(ctx, ec.Client, owner, repo, insecureOpts); err != nil {
			logger.Err(err).Msg("Failed to post insecure check run")
		}
	}
}

// isTransientClientError returns true when err is a GitHub API or network error
// that should not overwrite the user-visible check status. Policy-logic errors
// (parse failures, unexpected evaluation states) return false.
func isTransientClientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		return ghErr.Response != nil && ghErr.Response.StatusCode >= 400
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return strings.Contains(err.Error(), "non-200 OK status code")
}
