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
	"math/rand"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/reviewer"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func (ec *EvalContext) requestReviewsForResult(ctx context.Context, trigger common.Trigger, result common.Result) error {
	logger := zerolog.Ctx(ctx)

	if ec.PullContext.IsDraft() || result.Status != common.StatusPending {
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
		return ec.requestReviews(ctx, reqs)
	}

	logger.Debug().Msgf("No pending rules have review requests enabled, skipping reviewer assignment")
	return nil
}

func (ec *EvalContext) requestReviews(ctx context.Context, reqs []*common.Result) error {
	const maxDelayMillis = 2500

	logger := zerolog.Ctx(ctx)

	// Seed the random source with the PR creation time so that repeated
	// evaluations produce the same set of reviewers. This is required to avoid
	// duplicate requests on later evaluations.
	r := rand.New(rand.NewSource(ec.PullContext.CreatedAt().UnixNano()))
	selection, err := reviewer.SelectReviewers(ctx, ec.PullContext, reqs, r)
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
	reviewers, err := ec.PullContext.RequestedReviewers()
	if err != nil {
		return err
	}

	// After a user leaves a review, GitHub removes the user from the request
	// list. If the review didn't actually change the state of the requesting
	// rule, policy-bot may request the same user again. To avoid this, include
	// any reviews on the head commit in the set of existing reviewers to avoid
	// re-requesting them until the content changes.
	head := ec.PullContext.HeadSHA()
	reviews, err := ec.PullContext.Reviews()
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

		owner := ec.PullContext.RepositoryOwner()
		name := ec.PullContext.RepositoryName()
		number := ec.PullContext.Number()

		_, _, err = ec.Client.PullRequests.RequestReviewers(ctx, owner, name, number, req)
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
