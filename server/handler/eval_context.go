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
	"slices"
	"strings"

	"github.com/google/go-github/v93/github"
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

	// RulesetChecks finds the ruleset checks to post when ruleset status
	// checks are enabled.
	RulesetChecks RulesetChecks

	PullContext pull.Context
	Config      FetchedConfig

	// If true, store statuses in the Status and StatusContexts fields instead
	// of posting them to GitHub. Only the last status is saved, so when this
	// option is enabled, callers should check for a non-nil status after each
	// method call. Status.Context is the first of StatusContexts, or nil if
	// there are no contexts to post.
	SkipPostStatus bool
	Status         *github.RepoStatus
	StatusContexts []string
}

// Evaluate runs the full process for evaluating a pull request.
func (ec *EvalContext) Evaluate(ctx context.Context, trigger common.Trigger) error {
	return ec.evaluate(ctx, trigger, false)
}

// EvaluateBaseChange evaluates a pull request whose base branch changed. When
// ruleset checks are enabled, it first posts a pending status: ruleset checks
// are shared by all branches in a ruleset, so a check posted for the previous
// base may already be successful for the new base until evaluation completes.
func (ec *EvalContext) EvaluateBaseChange(ctx context.Context) error {
	return ec.evaluate(ctx, common.TriggerAll, ec.Options.HasStatusCheck(StatusCheckRuleset))
}

func (ec *EvalContext) evaluate(ctx context.Context, trigger common.Trigger, postPending bool) error {
	evaluator, err := ec.ParseConfig(ctx, trigger)
	if err != nil {
		return err
	}
	if evaluator == nil {
		return nil
	}

	if postPending {
		ec.PostStatus(ctx, "pending", "Re-evaluating after base branch change")
	}

	result, err := ec.EvaluatePolicy(ctx, evaluator)
	if err != nil {
		return err
	}

	ec.RunPostEvaluateActions(ctx, result, trigger)
	return nil
}

// ParseConfig reports policy loading or validation errors and returns the policy
// if it exists, is valid, and requires evaluation for the trigger.
func (ec *EvalContext) ParseConfig(ctx context.Context, trigger common.Trigger) (*policy.ParsedPolicy, error) {
	logger := zerolog.Ctx(ctx)

	fc := ec.Config
	switch {
	case fc.LoadError != nil:
		logger.Warn().Err(fc.LoadError).Bool("seen_policy", fc.SeenPolicy).Msgf("Error loading %s", fc.PolicyLocation())

		// If policy-bot has never seen a policy file for this base branch
		// then suppress the failing status, except for shared ruleset checks.
		msg := fmt.Sprintf("Error loading policy from %s", fc.Sources[0].Source)
		if fc.SeenPolicy {
			ec.PostStatus(ctx, "error", msg)
		} else {
			ec.postRulesetError(ctx, msg)
		}
		return nil, errors.Wrapf(fc.LoadError, "failed to load %s", fc.PolicyLocation())

	case fc.ParseError != nil:
		logger.Warn().Err(fc.ParseError).Msg("Invalid policy configuration")

		ec.PostStatus(ctx, "error", fmt.Sprintf("Invalid policy %s", fc.Sources[0]))
		return nil, errors.WithStack(fc.ParseError)

	case fc.Policy == nil:
		logger.Debug().Msg("No policy defined for repository")
		base, _ := ec.PullContext.Branches()
		ec.postRulesetError(ctx, fmt.Sprintf("No policy found for %s", base))
		return nil, nil
	}

	policyTrigger := fc.Policy.Trigger()
	if !trigger.Matches(policyTrigger) {
		logger.Debug().
			Str("event_trigger", trigger.String()).
			Str("policy_trigger", policyTrigger.String()).
			Msg("No evaluation necessary for this trigger, skipping")
		return nil, nil
	}
	return fc.Policy, nil
}

// EvaluatePolicy evaluates the policy for a PR and generates a result. The
// evaluator must be non-nil, meaning callers should check the output of
// ParseConfig before calling this method.
func (ec *EvalContext) EvaluatePolicy(ctx context.Context, evaluator common.Evaluator) (common.Result, error) {
	logger := zerolog.Ctx(ctx)

	result := evaluator.Evaluate(ctx, ec.PullContext)
	if result.Error != nil {
		source := ec.Config.Sources[0]
		msg := fmt.Sprintf("Error evaluating policy in %s: %s", source.Source, source.Path)
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

// PostStatus posts a status for the evaluated PR to every enabled status
// context. If there are no contexts, nothing is posted.
func (ec *EvalContext) PostStatus(ctx context.Context, state, message string) {
	logger := zerolog.Ctx(ctx)

	if !ec.SkipPostStatus && !ec.PullContext.IsOpen() {
		logger.Info().Msg("Skipping status update because PR state is not open")
		return
	}

	contexts, err := ec.statusContexts(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to find ruleset status checks")
		// if the ruleset lookup fails, post a non-success result to the ruleset checks previously on the commit if
		// they exist to prevent stale successes sticking around
		if !ec.SkipPostStatus && state != "success" {
			contexts = ec.addPreviousRulesetContexts(ctx, contexts)
		}
	}

	ec.postToContexts(ctx, state, message, contexts)
}

// postRulesetError posts an error to the ruleset checks of a base branch that
// has no usable policy. Ruleset checks are shared by all branches in a ruleset,
// so otherwise a success posted for another branch on the same commit could
// satisfy this one. The per-branch check is not posted.
func (ec *EvalContext) postRulesetError(ctx context.Context, message string) {
	logger := zerolog.Ctx(ctx)

	if !ec.Options.HasStatusCheck(StatusCheckRuleset) {
		return
	}
	if !ec.SkipPostStatus && !ec.PullContext.IsOpen() {
		logger.Info().Msg("Skipping status update because PR state is not open")
		return
	}

	contexts, err := ec.rulesetContexts(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to find ruleset status checks")
		if !ec.SkipPostStatus {
			contexts = ec.addPreviousRulesetContexts(ctx, contexts)
		}
	}
	if len(contexts) == 0 {
		return
	}

	ec.postToContexts(ctx, "error", message, contexts)
}

// postToContexts posts a status for the evaluated PR to each context, or
// stores it if SkipPostStatus is set.
func (ec *EvalContext) postToContexts(ctx context.Context, state, message string, contexts []string) {
	logger := zerolog.Ctx(ctx)

	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	sha := ec.PullContext.HeadSHA()
	base, _ := ec.PullContext.Branches()

	publicURL := strings.TrimSuffix(ec.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, ec.PullContext.Number())

	status := github.RepoStatus{
		State:       &state,
		Description: &message,
		TargetURL:   &detailsURL,
	}

	if ec.SkipPostStatus {
		if len(contexts) > 0 {
			status.Context = new(contexts[0])
		}
		ec.Status = &status
		ec.StatusContexts = contexts
		return
	}

	if len(contexts) == 0 {
		logger.Debug().Msgf("No status checks to post for base branch %q", base)
		return
	}

	for _, c := range contexts {
		status.Context = new(c)
		if err := PostStatus(ctx, ec.Client, owner, repo, sha, status); err != nil {
			logger.Err(err).Msg("Failed to post repo status")
		}
	}
	if ec.Options.PostInsecureStatusChecks {
		status.Context = new(ec.Options.StatusCheckContext)
		if err := PostStatus(ctx, ec.Client, owner, repo, sha, status); err != nil {
			logger.Err(err).Msg("Failed to post insecure repo status")
		}
	}
}

// statusContexts returns the de-duplicated status contexts to post for the pull
// request, based on the enabled status check types. If the ruleset lookup
// fails, the returned contexts exclude all ruleset checks and the error is
// returned with them.
func (ec *EvalContext) statusContexts(ctx context.Context) ([]string, error) {
	base, _ := ec.PullContext.Branches()

	var contexts []string
	if ec.Options.HasStatusCheck(StatusCheckBranch) {
		contexts = append(contexts, ec.Options.BranchStatusContext(base))
	}
	if ec.Options.HasStatusCheck(StatusCheckRuleset) {
		checks, err := ec.rulesetContexts(ctx)
		if err != nil {
			return contexts, err
		}
		for _, c := range checks {
			if !slices.Contains(contexts, c) {
				contexts = append(contexts, c)
			}
		}
	}
	return contexts, nil
}

// rulesetContexts returns the ruleset checks to post for the pull request's
// base branch.
func (ec *EvalContext) rulesetContexts(ctx context.Context) ([]string, error) {
	if ec.RulesetChecks == nil {
		return nil, errors.New("ruleset status checks are enabled but no ruleset lookup is configured")
	}
	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	base, _ := ec.PullContext.Branches()
	return ec.RulesetChecks.RequiredChecks(ctx, ec.Client, owner, repo, base)
}

// addPreviousRulesetContexts adds the ruleset contexts that already have a
// status on the head commit. It is used when the ruleset lookup fails: a
// success can't be posted without the rules, but any other state can safely
// replace previous ruleset checks, so that a disapproval or revoked approval
// does not leave a stale success. Statuses are matched by prefix only, because
// posting a state other than success is safe regardless of who posted before.
func (ec *EvalContext) addPreviousRulesetContexts(ctx context.Context, contexts []string) []string {
	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	sha := ec.PullContext.HeadSHA()
	prefix := RulesetContext(ec.Options.StatusCheckContext, "")

	var previous []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		combined, res, err := ec.Client.Repositories.GetCombinedStatus(ctx, owner, repo, sha, opts)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("Failed to find previous ruleset status checks")
			return contexts
		}
		for _, s := range combined.Statuses {
			if c := s.GetContext(); strings.HasPrefix(c, prefix) {
				previous = append(previous, c)
			}
		}
		if res.NextPage == 0 {
			break
		}
		opts.Page = res.NextPage
	}

	slices.Sort(previous)
	for _, c := range previous {
		if !slices.Contains(contexts, c) {
			contexts = append(contexts, c)
		}
	}
	return contexts
}
