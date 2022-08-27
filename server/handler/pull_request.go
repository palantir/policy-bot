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

type PullRequest struct {
	Base
}

func (h *PullRequest) Handles() []string { return []string{"pull_request"} }

// Handle pull_request
// https://developer.github.com/v3/activity/events/types/#requestevent
func (h *PullRequest) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.PullRequestEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse pull request event payload")
	}

	repo := event.GetRepo()
	owner := repo.GetOwner().GetLogin()
	number := event.GetNumber()
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
	pr, _, err := client.PullRequests.Get(ctx, owner, repo.GetName(), number)
	if err != nil {
		return errors.Wrapf(err, "failed to get pull request %s/%s#%d", owner, repo.GetName(), number)
	}

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

	var t common.Trigger
	switch event.GetAction() {
	case "opened", "reopened", "ready_for_review":
		t = common.TriggerCommit | common.TriggerPullRequest
	case "synchronize":
		err := h.dismissStaleReviews(ctx, prctx, client, fc.Config.ApprovalRules)
		if err != nil {
			return err
		}
		t = common.TriggerCommit
	case "edited":
		t = common.TriggerPullRequest
	case "labeled", "unlabeled":
		t = common.TriggerLabel
	default:
		return nil
	}

	return h.Evaluate(ctx, installationID, t, pull.Locator{
		Owner:  event.GetRepo().GetOwner().GetLogin(),
		Repo:   event.GetRepo().GetName(),
		Number: event.GetPullRequest().GetNumber(),
		Value:  event.GetPullRequest(),
	})
}

func (h *PullRequest) dismissStaleReviews(ctx context.Context, prctx pull.Context, client *github.Client, rules []*approval.Rule) error {
	for _, r := range rules {
		if !r.Options.InvalidateOnPush {
			continue
		}

		_, invalidatedCandidates, err := r.FilteredCandidates(ctx, prctx)
		if err != nil {
			return err
		}

		for _, c := range invalidatedCandidates {
			if c.Type != common.ReviewCandidate {
				continue
			}

			review, err := h.getReviewByID(prctx, c.ID)
			if err != nil {
				return err
			}

			if review.State != "APPROVED" {
				continue
			}

			repo := prctx.RepositoryName()
			owner := prctx.RepositoryOwner()
			number := prctx.Number()
			dismissalRequest := &github.PullRequestReviewDismissalRequest{}
			_, _, err = client.PullRequests.DismissReview(ctx, owner, repo, number, review.ID, dismissalRequest)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *PullRequest) getReviewByID(prctx pull.Context, id int64) (*pull.Review, error) {
	reviews, err := prctx.Reviews()
	if err != nil {
		return nil, err
	}

	for _, r := range reviews {
		if r.ID == id {
			return r, nil
		}
	}

	return nil, nil
}
