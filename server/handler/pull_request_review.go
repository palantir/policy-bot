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

	"github.com/google/go-github/v45/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/approval"
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

	pr := event.GetPullRequest()
	repo := event.GetRepo()
	owner := repo.GetOwner().GetLogin()
	number := event.GetPullRequest().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	v4client, err := h.NewInstallationV4Client(installationID)
	if err != nil {
		return err
	}

	ctx, _ = h.PreparePRContext(ctx, installationID, event.GetPullRequest())

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, owner, h.Installations, h.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, pull.Locator{
		Owner:  owner,
		Repo:   repo.GetName(),
		Number: number,
		Value:  pr,
	})
	if err != nil {
		return err
	}

	fc := h.ConfigFetcher.ConfigForPR(ctx, prctx, client)

	evaluator, err := h.Base.ValidateFetchedConfig(ctx, prctx, client, fc, common.TriggerReview)
	if err != nil {
		return err
	}
	if evaluator == nil {
		return nil
	}

	reviewState := pull.ReviewState(event.GetReview().GetState())
	if !h.affectsApproval(reviewState, fc.Config.ApprovalRules) {
		return nil
	}

	_, err = h.Base.EvaluateFetchedConfig(ctx, prctx, client, evaluator, fc)
	if err != nil {
		return err
	}

	return nil
}

func (h *PullRequestReview) affectsApproval(reviewState pull.ReviewState, rules []*approval.Rule) bool {
	for _, rule := range rules {
		if reviewState == rule.Options.GetMethods().GithubReviewState {
			return true
		}
	}

	return false
}
