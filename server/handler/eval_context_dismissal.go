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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
)

func (ec *EvalContext) dismissStaleReviewsForResult(ctx context.Context, result common.Result) error {
	logger := zerolog.Ctx(ctx)

	approvers := findAllApprovers(&result)

	alreadyDismissed := make(map[string]bool)
	for _, d := range findAllDismissals(&result) {
		// Only dismiss stale reviews for now (ignore comments)
		if d.Candidate.Type != common.ReviewCandidate {
			continue
		}
		// Only dismiss reviews from users who are not currently approvers
		if approvers[d.Candidate.User] {
			continue
		}
		// Only dismiss reviews once if they're dismissed by multiple rules
		if alreadyDismissed[d.Candidate.ReviewID] {
			continue
		}

		logger.Info().Str("reason", d.Reason).Msgf("Dismissing stale review %s", d.Candidate.ReviewID)
		if err := dismissPullRequestReview(ctx, ec.V4Client, d.Candidate.ReviewID, d.Reason); err != nil {
			return err
		}
		alreadyDismissed[d.Candidate.ReviewID] = true
	}

	return nil
}

func findAllDismissals(result *common.Result) []*common.Dismissal {
	var dismissals []*common.Dismissal

	if len(result.Children) == 0 && result.Error == nil {
		dismissals = append(dismissals, result.Dismissals...)
	}
	for _, c := range result.Children {
		dismissals = append(dismissals, findAllDismissals(c)...)
	}

	return dismissals
}

func findAllApprovers(result *common.Result) map[string]bool {
	approvers := make(map[string]bool)

	if len(result.Children) == 0 && result.Error == nil && result.Approvers != nil {
		for _, a := range result.Approvers.Actors {
			approvers[a.User] = true
		}
	}
	for _, c := range result.Children {
		for u := range findAllApprovers(c) {
			approvers[u] = true
		}
	}

	return approvers
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
