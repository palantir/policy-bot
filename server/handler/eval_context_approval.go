// Copyright 2026 Palantir Technologies, Inc.
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

	"github.com/google/go-github/v93/github"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	reviewStateApproved = "APPROVED"
	reviewEventApprove  = "APPROVE"
)

// syncApprovalReview makes the bot's pull request review match the status
// that was just posted. A "success" status results in an approving review on
// the head commit; any other state dismisses the bot's approving reviews so
// that a stale approval can never outlive a failing, pending, or erroring
// policy.
func (ec *EvalContext) syncApprovalReview(ctx context.Context, state, message string) error {
	logger := zerolog.Ctx(ctx)

	if ec.AppName == "" {
		return errors.New("app name is unknown, cannot identify the bot's reviews")
	}
	botLogin := ec.AppName + "[bot]"

	owner := ec.PullContext.RepositoryOwner()
	repo := ec.PullContext.RepositoryName()
	number := ec.PullContext.Number()
	sha := ec.PullContext.HeadSHA()

	approvals, err := ec.listApprovalsBy(ctx, owner, repo, number, botLogin)
	if err != nil {
		return err
	}

	if state == "success" {
		for _, r := range approvals {
			if r.GetCommitID() == sha {
				logger.Debug().Msg("Approval review already exists for head commit")
				return nil
			}
		}

		logger.Info().Msgf("Submitting approval review on %s", sha)
		_, _, err := ec.Client.PullRequests.CreateReview(ctx, owner, repo, number, &github.PullRequestReviewRequest{
			CommitID: github.Ptr(sha),
			Body:     github.Ptr(fmt.Sprintf("Approved by policy-bot: %s", message)),
			Event:    github.Ptr(reviewEventApprove),
		})
		return errors.Wrap(err, "failed to submit approval review")
	}

	for _, r := range approvals {
		logger.Info().Msgf("Dismissing approval review %d because status is %s", r.GetID(), state)
		_, _, err := ec.Client.PullRequests.DismissReview(ctx, owner, repo, number, r.GetID(), github.PullRequestDismissReviewRequest{
			Message: fmt.Sprintf("Policy is no longer approved (%s): %s", state, message),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to dismiss approval review %d", r.GetID())
		}
	}
	return nil
}

// listApprovalsBy returns the active (not dismissed) approving reviews that
// the given user has submitted on a pull request.
func (ec *EvalContext) listApprovalsBy(ctx context.Context, owner, repo string, number int, login string) ([]*github.PullRequestReview, error) {
	var approvals []*github.PullRequestReview

	opts := &github.ListOptions{PerPage: 100}
	for {
		reviews, res, err := ec.Client.PullRequests.ListReviews(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list pull request reviews")
		}
		for _, r := range reviews {
			if r.GetUser().GetLogin() == login && r.GetState() == reviewStateApproved {
				approvals = append(approvals, r)
			}
		}
		if res.NextPage == 0 {
			return approvals, nil
		}
		opts.Page = res.NextPage
	}
}

// excludeReviewsBy wraps a pull.Context so that reviews submitted by the given
// user are hidden. policy-bot uses this to make sure its own approval reviews
// are never counted as approvals when evaluating a policy.
func excludeReviewsBy(prctx pull.Context, login string) pull.Context {
	return &reviewFilterContext{Context: prctx, excluded: login}
}

type reviewFilterContext struct {
	pull.Context
	excluded string
}

func (c *reviewFilterContext) Reviews() ([]*pull.Review, error) {
	reviews, err := c.Context.Reviews()
	if err != nil {
		return nil, err
	}

	filtered := make([]*pull.Review, 0, len(reviews))
	for _, r := range reviews {
		if r.Author != c.excluded {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}
