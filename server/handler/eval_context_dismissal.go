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
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
)

func (ec *EvalContext) dismissStaleReviewsForResult(ctx context.Context, result common.Result) error {
	logger := zerolog.Ctx(ctx)

	var allowedCandidates []*common.Candidate

	results := findResultsWithAllowedCandidates(&result)
	for _, res := range results {
		for _, candidate := range res.AllowedCandidates {
			if candidate.Type != common.ReviewCandidate {
				continue
			}
			allowedCandidates = append(allowedCandidates, candidate)
		}
	}

	reviews, err := ec.PullContext.Reviews()
	if err != nil {
		return err
	}

	for _, r := range reviews {
		if r.State != pull.ReviewApproved {
			continue
		}

		if reviewIsAllowed(r, allowedCandidates) {
			continue
		}

		reason := reasonForDismissedReview(r)
		if reason == "" {
			continue
		}

		message := fmt.Sprintf("Dismissed because the approval %s", reason)
		logger.Info().Msgf("Dismissing stale review %s because it %s", r.ID, reason)
		if err := dismissPullRequestReview(ctx, ec.V4Client, r.ID, message); err != nil {
			return err
		}
	}

	return nil
}

func findResultsWithAllowedCandidates(result *common.Result) []*common.Result {
	var results []*common.Result
	for _, c := range result.Children {
		results = append(results, findResultsWithAllowedCandidates(c)...)
	}

	if len(result.Children) == 0 && len(result.AllowedCandidates) > 0 && result.Error == nil {
		results = append(results, result)
	}

	return results
}

func reviewIsAllowed(review *pull.Review, allowedCandidates []*common.Candidate) bool {
	for _, candidate := range allowedCandidates {
		if review.ID == candidate.ReviewID {
			return true
		}
	}
	return false
}

// We already know that these are discarded review candidates for 1 of 2 reasons
// so first we check for edited and then we check to see if its a review thats at least
// 5 seconds old and we know that it was invalidated by a new commit.
//
// This is brittle and may need refactoring in future versions because it assumes the bot
// will take less than 5 seconds to respond, but thought that having a dismissal reason
// was valuable.
func reasonForDismissedReview(review *pull.Review) string {
	if !review.LastEditedAt.IsZero() {
		return "was edited"
	}

	if review.CreatedAt.Before(time.Now().Add(-5 * time.Second)) {
		return "was invalidated by another commit"
	}

	return ""
}

func dismissPullRequestReview(ctx context.Context, v4client *githubv4.Client, reviewID string, message string) error {
	var m struct {
		DismissPullRequestReview struct {
			ClientMutationID *githubv4.String
		} `graphql:"dismissPullRequestReview(input: $input)"`
	}

	input := githubv4.DismissPullRequestReviewInput{
		PullRequestReviewID: githubv4.String(reviewID),
		Message:             githubv4.String(message),
	}

	if err := v4client.Mutate(ctx, &m, input, nil); err != nil {
		return errors.Wrapf(err, "failed to dismiss pull request review %s", reviewID)
	}

	return nil
}
