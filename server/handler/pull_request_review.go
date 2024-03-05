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

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type PullRequestReview struct {
	Base
}

func (h *PullRequestReview) Handles() []string { return []string{"pull_request_review"} }

// Handle pull_request_review
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewevent
func (h *PullRequestReview) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.PullRequestReviewEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse pull request review event payload")
	}

	// Ignore events triggered by policy-bot (e.g. for dismissing stale reviews)
	if event.GetSender().GetLogin() == h.AppName+"[bot]" {
		return nil
	}

	pr := event.GetPullRequest()
	repo := event.GetRepo()
	owner := repo.GetOwner().GetLogin()
	number := event.GetPullRequest().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

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
		return nil
	}

	reviewState := pull.ReviewState(event.GetReview().GetState())
	if !h.affectsApproval(reviewState, evalCtx.Config.Config) {
		logger.Debug().Msg("Skipping evaluation because this review does not impact approval")
		return nil
	}

	result, err := evalCtx.EvaluatePolicy(ctx, evaluator)
	if err != nil {
		return err
	}

	evalCtx.RunPostEvaluateActions(ctx, result, common.TriggerReview)
	return nil
}

func (h *PullRequestReview) affectsApproval(reviewState pull.ReviewState, config *policy.Config) bool {
	states := make(map[pull.ReviewState]struct{})
	for _, rule := range config.ApprovalRules {
		states[rule.Options.GetMethods().GithubReviewState] = struct{}{}
	}
	if disapproval := config.Policy.Disapproval; disapproval != nil {
		states[disapproval.Options.GetDisapproveMethods().GithubReviewState] = struct{}{}
		states[disapproval.Options.GetRevokeMethods().GithubReviewState] = struct{}{}
	}

	for state := range states {
		if state == reviewState {
			return true
		}
	}
	return false
}
