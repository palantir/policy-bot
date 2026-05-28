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
	"encoding/json"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

type PullRequestReview struct {
	Base
}

func (h *PullRequestReview) Handles() []string { return []string{"pull_request_review"} }

// Handle pull_request_review
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewevent
func (h *PullRequestReview) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) (err error) {
	ctx, span := StartWebhookSpan(ctx, eventType, deliveryID)
	defer span.End()
	defer RecordError(span, &err)

	var event github.PullRequestReviewEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse pull request review event payload")
	}

	sender := event.GetSender().GetLogin()
	span.SetAttributes(
		attribute.String(AttrEventAction, event.GetAction()),
		attribute.String(AttrSenderLogin, sender),
		attribute.String(AttrReviewState, event.GetReview().GetState()),
		attribute.String(AttrReviewer, event.GetReview().GetUser().GetLogin()),
	)

	// Ignore events triggered by policy-bot (e.g. for dismissing stale reviews)
	if sender == h.AppName+"[bot]" {
		span.SetAttributes(attribute.String(AttrPolicySkipReason, SkipReasonSelfSender))
		return nil
	}

	pr := event.GetPullRequest()
	repo := event.GetRepo()
	owner := repo.GetOwner().GetLogin()
	number := event.GetPullRequest().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	span.SetAttributes(RepoAttrs(owner, repo.GetName())...)
	span.SetAttributes(
		attribute.Int(AttrPRNumber, number),
		attribute.String(AttrSHA, pr.GetHead().GetSHA()),
		attribute.Int64(AttrInstallationID, installationID),
	)

	ctx, logger := h.PreparePRContext(ctx, installationID, pr)

	evalCtx, err := h.NewEvalContext(ctx, installationID, pull.Locator{
		Owner:  owner,
		Repo:   repo.GetName(),
		Number: number,
		Value:  pr,
	})
	if err != nil {
		return err
	}

	evaluator, err := evalCtx.ParseConfig(ctx, common.TriggerReview)
	if err != nil {
		return err
	}
	if evaluator == nil {
		span.SetAttributes(attribute.String(AttrPolicySkipReason, SkipReasonNoPolicy))
		return nil
	}

	if !h.affectsApproval(event.GetReview(), evalCtx.Config.Config) {
		logger.Debug().Msg("Skipping evaluation because this review does not impact approval")
		span.SetAttributes(attribute.String(AttrPolicySkipReason, SkipReasonReviewDoesNotAffectApproval))
		return nil
	}

	result, err := evalCtx.EvaluatePolicy(ctx, evaluator)
	if err != nil {
		return err
	}

	evalCtx.RunPostEvaluateActions(ctx, result, common.TriggerReview)
	return nil
}

func (h *PullRequestReview) affectsApproval(review *github.PullRequestReview, config *policy.Config) bool {
	// States contains the review states that may affect approval. Always consider dismissed
	// reviews because they can revert the overall approval or disapproval to a previous state.
	states := map[pull.ReviewState]bool{
		pull.ReviewDismissed: true,
	}

	var methods []*common.Methods
	for _, rule := range config.ApprovalRules {
		m := rule.Options.GetMethods()
		states[m.GithubReviewState] = true
		methods = append(methods, m)
	}
	if disapproval := config.Policy.Disapproval; disapproval != nil {
		md := disapproval.Options.GetDisapproveMethods()
		states[md.GithubReviewState] = true
		methods = append(methods, md)

		mr := disapproval.Options.GetRevokeMethods()
		states[mr.GithubReviewState] = true
		methods = append(methods, mr)
	}

	reviewState := pull.ReviewState(review.GetState())
	if states[reviewState] {
		return true
	}

	// Reviews with the COMMENTED state are also processed as normal comments, so also consider
	// methods that match on comments in this case.
	if reviewState == pull.ReviewCommented {
		for _, m := range methods {
			if m.CommentMatches(review.GetBody()) {
				return true
			}
		}
	}

	return false
}
