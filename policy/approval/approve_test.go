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

package approval

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](val T) *T {
	return &val
}

var defaultOptions = Options{
	Methods: DefaultMethods(),
}

func TestIsApproved(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	ctx := logger.WithContext(context.Background())

	now := time.Now()
	basePullContext := func() *pulltest.Context {
		return &pulltest.Context{
			AuthorValue: "mhaypenny",
			BodyValue: &pull.Body{
				Body:         "/no-platform",
				CreatedAt:    now.Add(10 * time.Second),
				LastEditedAt: now.Add(20 * time.Second),
				Author:       pull.NewAuthor("body-editor"),
			},
			CommentsValue: []*pull.Comment{
				{
					CreatedAt:    now.Add(10 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("other-user"),
					Body:         "Why did you do this?",
				},
				{
					CreatedAt:    now.Add(20 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("comment-approver"),
					Body:         "LGTM :+1: :shipit:",
				},
				{
					CreatedAt:    now.Add(30 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("disapprover"),
					Body:         "I don't like things! :-1:",
				},
				{
					CreatedAt:    now.Add(40 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("mhaypenny"),
					Body:         ":+1: my stuff is cool",
				},
				{
					CreatedAt:    now.Add(50 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("contributor-author"),
					Body:         ":+1: I added to this PR",
				},
				{
					CreatedAt:    now.Add(60 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("contributor-committer"),
					Body:         ":+1: I also added to this PR",
				},
				{
					CreatedAt:    now.Add(70 * time.Second),
					LastEditedAt: now.Add(71 * time.Second),
					Author:       pull.NewAuthor("comment-editor"),
					Body:         "LGTM :+1: :shipit:",
				},
			},
			ReviewsValue: []*pull.Review{
				{
					CreatedAt:    now.Add(70 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("disapprover"),
					State:        pull.ReviewChangesRequested,
					Body:         "I _really_ don't like things!",
				},
				{
					CreatedAt:    now.Add(80 * time.Second),
					LastEditedAt: time.Time{},
					Author:       pull.NewAuthor("review-approver"),
					State:        pull.ReviewApproved,
					Body:         "I LIKE THIS",
				},
				{
					CreatedAt:    now.Add(90 * time.Second),
					LastEditedAt: now.Add(91 * time.Second),
					Author:       pull.NewAuthor("review-comment-editor"),
					State:        pull.ReviewApproved,
					Body:         "I LIKE THIS",
				},
			},
			HeadSHAValue: "97d5ea26da319a987d80f6db0b7ef759f2f2e441",
			CommitsValue: []*pull.Commit{
				{
					SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
					Author:    "mhaypenny",
					Committer: "mhaypenny",
				},
				{
					SHA:       "674832587eaaf416371b30f5bc5a47e377f534ec",
					Author:    "contributor-author",
					Committer: "mhaypenny",
					Parents:   []string{"c6ade256ecfc755d8bc877ef22cc9e01745d46bb"},
				},
				{
					SHA:       "97d5ea26da319a987d80f6db0b7ef759f2f2e441",
					Author:    "mhaypenny",
					Committer: "contributor-committer",
					Parents:   []string{"674832587eaaf416371b30f5bc5a47e377f534ec"},
				},
			},
			OrgMemberships: map[string][]string{
				"mhaypenny":             {"everyone"},
				"contributor-author":    {"everyone"},
				"contributor-committer": {"everyone"},
				"comment-approver":      {"everyone", "cool-org"},
				"review-approver":       {"everyone", "even-cooler-org"},
			},
			LatestStatusesValue: map[string]string{
				"build":  "success",
				"deploy": "pending",
				"scan":   "success",
			},
		}
	}

	assertApproved := func(t *testing.T, prctx pull.Context, r *Rule, expected string) {
		allowedCandidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, allowedCandidates)
		require.NoError(t, err)

		if assert.True(t, approved, "pull request was not approved") {
			msg := statusDescription(approved, result, allowedCandidates)
			assert.Equal(t, expected, msg)
		}
	}

	assertPending := func(t *testing.T, prctx pull.Context, r *Rule, expected string) {
		allowedCandidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, allowedCandidates)
		require.NoError(t, err)

		if assert.False(t, approved, "pull request was incorrectly approved") {
			msg := statusDescription(approved, result, allowedCandidates)
			assert.Equal(t, expected, msg)
		}
	}

	t.Run("noApprovalRequired", func(t *testing.T) {
		prctx := basePullContext()
		// There are no approvers required, so `Comments()` should not be
		// called, and therefore this error should not be returned. We are
		// checking that we don't make an unnecessary call to the GitHub API.
		prctx.CommentsError = errors.New("Comments() was called")

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
		}
		assertApproved(t, prctx, r, "No approval required")
	})

	t.Run("singleApprovalRequired", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 7 approvals from disqualified users")
	})

	t.Run("authorCannotApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowAuthor: ptr(false),
				Defaults:    &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, review-approver")
	})

	t.Run("authorCanApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowAuthor: ptr(true),
				Defaults:    &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, mhaypenny, review-approver")
	})

	t.Run("contributorsCannotApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowContributor: ptr(false),
				Defaults:         &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, review-approver")
	})

	t.Run("contributorsIncludingAuthorCanApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowContributor: ptr(true),
				AllowAuthor:      ptr(false),
				Defaults:         &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, mhaypenny, contributor-author, contributor-committer, review-approver")
	})

	t.Run("contributorsExcludingAuthorCanApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowNonAuthorContributor: ptr(true),
				Defaults:                  &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, contributor-author, contributor-committer, review-approver")
	})

	t.Run("nonAuthorContributorsAndAuthorCanApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowNonAuthorContributor: ptr(true),
				AllowAuthor:               ptr(true),
				Defaults:                  &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, mhaypenny, contributor-author, contributor-committer, review-approver")
	})

	t.Run("contributorsAndAuthorCanApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowNonAuthorContributor: ptr(true),
				AllowContributor:          ptr(true),
				Defaults:                  &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, mhaypenny, contributor-author, contributor-committer, review-approver")
	})

	t.Run("specificUserApproves", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r = &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"does-not-exist"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 7 approvals from disqualified users")
	})

	t.Run("specificOrgApproves", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"cool-org"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r = &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"does-not-exist", "other-org"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 7 approvals from disqualified users")
	})

	t.Run("specificOrgsOrUserApproves", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 2,
				Actors: common.Actors{
					Users:         []string{"review-approver"},
					Organizations: []string{"cool-org"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, review-approver")
	})

	t.Run("invalidateCommentOnPush", func(t *testing.T) {
		prctx := basePullContext()
		prctx.PushedAtValue = map[string]time.Time{
			"c6ade256ecfc755d8bc877ef22cc9e01745d46bb": now.Add(25 * time.Second),
		}
		prctx.HeadSHAValue = "c6ade256ecfc755d8bc877ef22cc9e01745d46bb"
		prctx.CommitsValue = []*pull.Commit{
			{
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r.Options.InvalidateOnPush = ptr(true)
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 6 approvals from disqualified users")
	})

	t.Run("invalidateReviewOnPush", func(t *testing.T) {
		prctx := basePullContext()
		prctx.PushedAtValue = map[string]time.Time{
			"c6ade256ecfc755d8bc877ef22cc9e01745d46bb": now.Add(85 * time.Second),
		}
		prctx.HeadSHAValue = "c6ade256ecfc755d8bc877ef22cc9e01745d46bb"
		prctx.CommitsValue = []*pull.Commit{
			{
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"review-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by review-approver")

		r.Options.InvalidateOnPush = ptr(true)
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 1 approval from disqualified users")
	})

	t.Run("ignoreUpdateMergeAfterReview", func(t *testing.T) {
		prctx := basePullContext()
		prctx.PushedAtValue = map[string]time.Time{
			"c6ade256ecfc755d8bc877ef22cc9e01745d46bb": now,
			"647c5078288f0ea9de27b5c280f25edaf2089045": now.Add(25 * time.Second),
		}
		prctx.HeadSHAValue = "647c5078288f0ea9de27b5c280f25edaf2089045"
		prctx.CommitsValue = append(prctx.CommitsValue[:1], &pull.Commit{
			SHA:             "647c5078288f0ea9de27b5c280f25edaf2089045",
			CommittedViaWeb: true,
			Parents: []string{
				"c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				"2e1b0bb6ab144bf7a1b7a1df9d3bdcb0fe85a206",
			},
			Author: "merge-committer",
		})

		r := &Rule{
			Options: Options{
				InvalidateOnPush: ptr(true),
				Defaults:         &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 6 approvals from disqualified users")

		r.Options.IgnoreUpdateMerges = ptr(true)
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreUpdateMergeContributor", func(t *testing.T) {
		prctx := basePullContext()
		prctx.HeadSHAValue = "647c5078288f0ea9de27b5c280f25edaf2089045"
		prctx.CommitsValue = append(prctx.CommitsValue[:1], &pull.Commit{
			SHA:             "647c5078288f0ea9de27b5c280f25edaf2089045",
			CommittedViaWeb: true,
			Parents: []string{
				"c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				"2e1b0bb6ab144bf7a1b7a1df9d3bdcb0fe85a206",
			},
			Author: "merge-committer",
		})
		prctx.CommentsValue = append(prctx.CommentsValue, &pull.Comment{
			CreatedAt: now.Add(100 * time.Second),
			Author:    pull.NewAuthor("merge-committer"),
			Body:      ":+1:",
		})

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"merge-committer"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 8 approvals from disqualified users")

		r.Options.IgnoreUpdateMerges = ptr(true)
		assertApproved(t, prctx, r, "Approved by merge-committer")
	})

	t.Run("ignoreCommits", func(t *testing.T) {
		prctx := basePullContext()
		prctx.HeadSHAValue = "ea9be5fcd016dc41d70dc457dfee2e64a8f951c1"
		prctx.CommitsValue = append(prctx.CommitsValue, &pull.Commit{
			SHA:       "ea9be5fcd016dc41d70dc457dfee2e64a8f951c1",
			Author:    "comment-approver",
			Committer: "comment-approver",
		})

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 7 approvals from disqualified users")

		r.Options.IgnoreCommitsBy = &common.Actors{
			Users: []string{"comment-approver"},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreCommitsMixedAuthorCommiter", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Options: Options{
				IgnoreCommitsBy: &common.Actors{
					Users: []string{"contributor-author"},
				},
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"contributor-author"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 7 approvals from disqualified users")
	})

	t.Run("ignoreCommitsInvalidateOnPush", func(t *testing.T) {
		prctx := basePullContext()
		prctx.PushedAtValue = map[string]time.Time{
			"c6ade256ecfc755d8bc877ef22cc9e01745d46bb": now.Add(25 * time.Second),
		}
		prctx.HeadSHAValue = "c6ade256ecfc755d8bc877ef22cc9e01745d46bb"
		prctx.CommitsValue = []*pull.Commit{
			{
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r.Options.InvalidateOnPush = ptr(true)
		r.Options.IgnoreCommitsBy = &common.Actors{
			Users: []string{"mhaypenny"},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreCommitsInvalidateOnPushBatches", func(t *testing.T) {
		prctx := basePullContext()
		prctx.PushedAtValue = map[string]time.Time{
			"584b9232835ae85a2d8216fb55fd9cad8389092c": now.Add(25 * time.Second),
			"7f4cd9d3999061605ce4cf261234e431b52e61ee": now,
		}
		prctx.HeadSHAValue = "584b9232835ae85a2d8216fb55fd9cad8389092c"
		prctx.CommitsValue = []*pull.Commit{
			{
				SHA:       "584b9232835ae85a2d8216fb55fd9cad8389092c",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
				Parents:   []string{"7f4cd9d3999061605ce4cf261234e431b52e61ee"},
			},
			{
				SHA:       "7f4cd9d3999061605ce4cf261234e431b52e61ee",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
				Parents:   []string{"b2b73abd18bbad3ee3c6d0055697177cf476bf67"},
			},
			{
				SHA:       "b2b73abd18bbad3ee3c6d0055697177cf476bf67",
				Author:    "nstrawnickel",
				Committer: "nstrawnickel",
			},
		}

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r.Options.InvalidateOnPush = ptr(true)
		assertPending(t, prctx, r, "0/1 required approvals. Ignored 6 approvals from disqualified users")

		r.Options.IgnoreCommitsBy = &common.Actors{
			Users: []string{"mhaypenny"},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreEditedReviewComments", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"review-comment-editor"},
				},
			},
		}

		assertApproved(t, prctx, r, "Approved by review-comment-editor")

		r.Options.IgnoreEditedComments = ptr(true)

		assertPending(t, prctx, r, "0/1 required approvals. Ignored 5 approvals from disqualified users")
	})

	t.Run("ignoreEditedComments", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-editor"},
				},
			},
		}

		assertApproved(t, prctx, r, "Approved by comment-editor")

		r.Options.IgnoreEditedComments = ptr(true)

		assertPending(t, prctx, r, "0/1 required approvals. Ignored 5 approvals from disqualified users")
	})

	t.Run("ignoreEditedCommentsWithBodyPattern", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					BodyPatterns: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("/no-platform")),
					},
				},
				IgnoreEditedComments: ptr(false),
				Defaults:             &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"body-editor"},
				},
			},
		}

		assertApproved(t, prctx, r, "Approved by body-editor")

		r.Options.IgnoreEditedComments = ptr(true)

		assertPending(t, prctx, r, "0/1 required approvals. Ignored 5 approvals from disqualified users")
	})

	t.Run("conditionsRequiredStatusPending", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Conditions: predicate.Predicates{
					HasStatus: &predicate.HasStatus{Statuses: []string{"deploy"}},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required conditions")
	})

	t.Run("conditionsRequiredStatusSuccess", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Conditions: predicate.Predicates{
					HasStatus: &predicate.HasStatus{Statuses: []string{"build"}},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by required conditions")
	})

	t.Run("conditionsRequiredStatusAndMissingApproval", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Conditions: predicate.Predicates{
					HasStatus: &predicate.HasStatus{Statuses: []string{"build"}},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 required approvals and 1/1 required conditions. Ignored 7 approvals from disqualified users")
	})

	t.Run("conditionsRequiredStatusAndOrgApproval", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
				Conditions: predicate.Predicates{
					HasStatus: &predicate.HasStatus{Statuses: []string{"build"}},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver, review-approver and required conditions")
	})
}

func TestTrigger(t *testing.T) {
	t.Run("triggerCommitOnRules", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerCommit), "expected %s to match %", r.Trigger(), common.TriggerCommit)
	})

	t.Run("triggerCommentOnComments", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					Comments: []string{
						"lgtm",
					},
				},
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerCommit), "expected %s to match %s", r.Trigger(), common.TriggerCommit)
		assert.True(t, r.Trigger().Matches(common.TriggerComment), "expected %s to match %s", r.Trigger(), common.TriggerComment)
	})

	t.Run("triggerCommentOnCommentPatterns", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					CommentPatterns: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("(?i)nice")),
					},
				},
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerCommit), "expected %s to match %s", r.Trigger(), common.TriggerCommit)
		assert.True(t, r.Trigger().Matches(common.TriggerComment), "expected %s to match %s", r.Trigger(), common.TriggerComment)
	})

	t.Run("triggerReviewForGithubReview", func(t *testing.T) {
		defaultGithubReview := true
		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					GithubReview: &defaultGithubReview,
				},
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerCommit), "expected %s to match %s", r.Trigger(), common.TriggerCommit)
		assert.True(t, r.Trigger().Matches(common.TriggerReview), "expected %s to match %s", r.Trigger(), common.TriggerReview)
	})

	t.Run("triggerReviewForGithubReviewCommentPatterns", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					GithubReviewCommentPatterns: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("(?i)nice")),
					},
				},
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerCommit), "expected %s to match %s", r.Trigger(), common.TriggerCommit)
		assert.True(t, r.Trigger().Matches(common.TriggerReview), "expected %s to match %s", r.Trigger(), common.TriggerReview)
	})

	t.Run("triggerPullRequestForBodyPatterns", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Methods: &common.Methods{
					BodyPatterns: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("(?i)nice")),
					},
				},
				IgnoreEditedComments: ptr(false),
				Defaults:             &defaultOptions,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerPullRequest), "expected %s to match %s", r.Trigger(), common.TriggerPullRequest)
	})

	t.Run("triggerStatusesForStatuses", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				Defaults: &defaultOptions,
			},
			Requires: Requires{
				Conditions: predicate.Predicates{
					HasStatus: predicate.NewHasStatus([]string{"status1"}, []string{"skipped", "success"}),
				},
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerStatus), "expected %s to match %s", r.Trigger(), common.TriggerStatus)
	})
}

func TestIsPendingOnConditionsOnly(t *testing.T) {
	tests := map[string]struct {
		rule     Rule
		result   common.RequiresResult
		expected bool
	}{
		"actors_not_approved_returns_false": {
			rule: Rule{},
			result: common.RequiresResult{
				ActorsApproved:     false,
				ConditionsApproved: false,
			},
			expected: false,
		},
		"conditions_already_approved_returns_false": {
			rule: Rule{},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: true,
			},
			expected: false,
		},
		"actors_approved_ci_status_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasStatus: predicate.NewHasStatus([]string{"ci/test"}, nil),
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false},
				},
			},
			expected: true,
		},
		"actors_approved_workflow_result_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: predicate.NewHasWorkflowResult([]string{"build"}, nil),
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false},
				},
			},
			expected: true,
		},
		"actors_approved_label_condition_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasLabels: &predicate.HasLabels{"approved"},
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false},
				},
			},
			expected: false,
		},
		"mixed_ci_and_label_conditions_one_unsatisfied_label": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasStatus: predicate.NewHasStatus([]string{"ci/test"}, nil),
						HasLabels: &predicate.HasLabels{"approved"},
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: true},  // CI passed
					{Satisfied: false}, // label missing
				},
			},
			expected: false,
		},
		"mixed_ci_and_label_conditions_only_ci_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasStatus: predicate.NewHasStatus([]string{"ci/test"}, nil),
						HasLabels: &predicate.HasLabels{"approved"},
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false}, // CI not done
					{Satisfied: true},  // label present
				},
			},
			expected: true,
		},
		"no_conditions_returns_false": {
			rule: Rule{},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: true,
			},
			expected: false,
		},
		"no_actor_requirements_ci_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasStatus: predicate.NewHasStatus([]string{"ci/test"}, nil),
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false},
				},
			},
			expected: true,
		},
		"multiple_ci_conditions_all_unsatisfied": {
			rule: Rule{
				Requires: Requires{
					Conditions: predicate.Predicates{
						HasStatus:         predicate.NewHasStatus([]string{"ci/test"}, nil),
						HasWorkflowResult: predicate.NewHasWorkflowResult([]string{"build"}, nil),
					},
				},
			},
			result: common.RequiresResult{
				ActorsApproved:     true,
				ConditionsApproved: false,
				Conditions: []*common.PredicateResult{
					{Satisfied: false},
					{Satisfied: false},
				},
			},
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := test.rule.isPendingOnConditionsOnly(test.result)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestSortCommits(t *testing.T) {
	tests := map[string]struct {
		Commits       []*pull.Commit
		Head          string
		ExpectedOrder []string
	}{
		"sorted": {
			Commits: []*pull.Commit{
				{SHA: "1", Parents: []string{"2"}},
				{SHA: "2", Parents: []string{"3"}},
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "5"},
			},
			Head:          "1",
			ExpectedOrder: []string{"1", "2", "3", "4", "5"},
		},
		"reverseSorted": {
			Commits: []*pull.Commit{
				{SHA: "5"},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "2", Parents: []string{"3"}},
				{SHA: "1", Parents: []string{"2"}},
			},
			Head:          "1",
			ExpectedOrder: []string{"1", "2", "3", "4", "5"},
		},
		"unsorted": {
			Commits: []*pull.Commit{
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "1", Parents: []string{"2"}},
				{SHA: "5"},
				{SHA: "2", Parents: []string{"3"}},
			},
			Head:          "1",
			ExpectedOrder: []string{"1", "2", "3", "4", "5"},
		},
		"partialOrder": {
			Commits: []*pull.Commit{
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "1", Parents: []string{"2"}},
				{SHA: "5"},
				{SHA: "2", Parents: []string{"3"}},
			},
			Head:          "3",
			ExpectedOrder: []string{"3", "4", "5"},
		},
		"independentHistory": {
			Commits: []*pull.Commit{
				{SHA: "1", Parents: []string{"2"}},
				{SHA: "2"},
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "5"},
			},
			Head:          "1",
			ExpectedOrder: []string{"1", "2"},
		},
		"missingHead": {
			Commits: []*pull.Commit{
				{SHA: "1", Parents: []string{"2"}},
				{SHA: "2", Parents: []string{"3"}},
				{SHA: "3", Parents: []string{"4"}},
				{SHA: "4", Parents: []string{"5"}},
				{SHA: "5"},
			},
			Head:          "42",
			ExpectedOrder: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var actual []string
			for _, c := range sortCommits(test.Commits, test.Head) {
				actual = append(actual, c.SHA)
			}
			assert.Equal(t, test.ExpectedOrder, actual, "incorrect commit order")
		})
	}
}

func TestCodeownerGroupApproval(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	ctx := logger.WithContext(context.Background())

	now := time.Now()

	t.Run("allGroupsApprovedBySingleReviewer", func(t *testing.T) {
		// Single reviewer is member of both team-a and team-c, satisfying both groups
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a", "org/team-c"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
					"c/file.go": {"@org/team-c"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Len(t, result.OwnershipGroups, 2)
		for _, g := range result.OwnershipGroups {
			assert.True(t, g.Satisfied, "group %s should be satisfied", g.Key)
		}
	})

	t.Run("partialGroupApproval", func(t *testing.T) {
		// reviewer1 is only in team-a, so team-b group is not satisfied
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
					"b/file.go": {"@org/team-b"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Len(t, result.OwnershipGroups, 2)

		// Check which group is satisfied
		groupByKey := make(map[string]common.OwnershipGroupResult)
		for _, g := range result.OwnershipGroups {
			groupByKey[g.Key] = g
		}
		assert.True(t, groupByKey["@org/team-a"].Satisfied)
		assert.False(t, groupByKey["@org/team-b"].Satisfied)
	})

	t.Run("multipleReviewersSatisfyAllGroups", func(t *testing.T) {
		// Two reviewers, each covering different groups
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a"},
				"reviewer2": {"org/team-b", "org/team-c"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
				{Author: pull.NewAuthor("reviewer2"), State: pull.ReviewApproved, CreatedAt: now.Add(time.Second)},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
					"b/file.go": {"@org/team-b"},
					"c/file.go": {"@org/team-c"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Len(t, result.OwnershipGroups, 3)
		for _, g := range result.OwnershipGroups {
			assert.True(t, g.Satisfied, "group %s should be satisfied", g.Key)
		}
	})

	t.Run("userCodeownerApproval", func(t *testing.T) {
		// Direct user codeowner (not team)
		prctx := &pulltest.Context{
			AuthorValue: "author",
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("johndoe"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"file.go": {"@johndoe"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Len(t, result.OwnershipGroups, 1)
		assert.True(t, result.OwnershipGroups[0].Satisfied)
		assert.Contains(t, result.OwnershipGroups[0].Approvers, "johndoe")
	})

	t.Run("noCodeownersFile", func(t *testing.T) {
		// No CODEOWNERS file - should pass (no codeowner requirements)
		prctx := &pulltest.Context{
			AuthorValue:     "author",
			CodeownersValue: nil, // No CODEOWNERS file
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Empty(t, result.OwnershipGroups)
	})

	t.Run("authorCannotApproveOwnGroup", func(t *testing.T) {
		// Author is a codeowner but shouldn't be able to approve (default behavior)
		prctx := &pulltest.Context{
			AuthorValue: "johndoe",
			TeamMemberships: map[string][]string{
				"johndoe": {"org/team-a"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("johndoe"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "johndoe"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods:     DefaultMethods(),
				AllowAuthor: ptr(false),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.False(t, approved)
		assert.Len(t, result.OwnershipGroups, 1)
		assert.False(t, result.OwnershipGroups[0].Satisfied)
	})

	t.Run("filesWithSameOwnersGroupedTogether", func(t *testing.T) {
		// Multiple files with the same owner should be in one group
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file1.go": {"@org/team-a"},
					"a/file2.go": {"@org/team-a"},
					"a/file3.go": {"@org/team-a"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		candidates, _, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, result, err := r.IsApproved(ctx, prctx, candidates)
		require.NoError(t, err)
		assert.True(t, approved)
		assert.Len(t, result.OwnershipGroups, 1) // All files in one group
		assert.Len(t, result.OwnershipGroups[0].Files, 3)
	})

	t.Run("statusDescriptionShowsOwnershipGroups", func(t *testing.T) {
		// Test status description for pending with ownership groups
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
					"b/file.go": {"@org/team-b"},
					"c/file.go": {"@org/team-c"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		result := r.Evaluate(ctx, prctx)
		assert.Equal(t, common.StatusPending, result.Status)
		assert.Contains(t, result.StatusDescription, "1/3 ownership groups covered")
	})

	t.Run("statusDescriptionShowsApprovedWithGroups", func(t *testing.T) {
		// Test status description for approved with ownership groups
		prctx := &pulltest.Context{
			AuthorValue: "author",
			TeamMemberships: map[string][]string{
				"reviewer1": {"org/team-a", "org/team-b"},
			},
			ReviewsValue: []*pull.Review{
				{Author: pull.NewAuthor("reviewer1"), State: pull.ReviewApproved, CreatedAt: now},
			},
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", Author: "author"},
			},
			HeadSHAValue: "abc123",
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"a/file.go": {"@org/team-a"},
					"b/file.go": {"@org/team-b"},
				},
			},
		}

		r := &Rule{
			Options: Options{
				Methods: DefaultMethods(),
			},
			Requires: Requires{
				Actors: common.Actors{Codeowners: true},
			},
		}

		result := r.Evaluate(ctx, prctx)
		assert.Equal(t, common.StatusApproved, result.Status)
		assert.Contains(t, result.StatusDescription, "Approved by reviewer1")
		assert.Contains(t, result.StatusDescription, "2 ownership groups")
	})

	// Shared ownership tests - validates "ANY" semantics where a file has multiple
	// owners and approval from any single owner should be sufficient
	sharedOwnershipTests := map[string]struct {
		reviewer        string
		teamMemberships []string
		owners          map[string][]string
		expectApproved  bool
		expectGroups    int
	}{
		"singleFileMultipleOwnersApprovedByFirstOwner": {
			reviewer:        "reviewer1",
			teamMemberships: []string{"org/infrastructure"},
			owners:          map[string][]string{"foo/bar.go": {"@org/infrastructure", "@org/team-a"}},
			expectApproved:  true,
			expectGroups:    1,
		},
		"singleFileMultipleOwnersApprovedBySecondOwner": {
			reviewer:        "reviewer1",
			teamMemberships: []string{"org/team-a"},
			owners:          map[string][]string{"foo/bar.go": {"@org/infrastructure", "@org/team-a"}},
			expectApproved:  true,
			expectGroups:    1,
		},
		"sharedOwnershipSingleApprover": {
			reviewer:        "reviewer1",
			teamMemberships: []string{"org/infrastructure"},
			owners: map[string][]string{
				"foo/bar.go": {"@org/infrastructure", "@org/team-a"},
				"baz.go":     {"@org/infrastructure"},
			},
			expectApproved: true,
			expectGroups:   2,
		},
		"sharedOwnershipPartialApprovalFails": {
			reviewer:        "reviewer1",
			teamMemberships: []string{"org/infrastructure"},
			owners: map[string][]string{
				"foo/bar.go": {"@org/infrastructure", "@org/team-a"},
				"baz.go":     {"@org/team-b"},
			},
			expectApproved: false,
			expectGroups:   2,
		},
		"sharedOwnershipDifferentCoOwners": {
			reviewer:        "reviewer1",
			teamMemberships: []string{"org/infrastructure"},
			owners: map[string][]string{
				"foo/bar.go": {"@org/infrastructure", "@org/team-a"},
				"baz/qux.go": {"@org/infrastructure", "@org/team-b"},
			},
			expectApproved: true,
			expectGroups:   2,
		},
		"sharedOwnershipWithUserAndTeam": {
			reviewer:        "johndoe",
			teamMemberships: nil,
			owners: map[string][]string{
				"foo/bar.go": {"@org/infrastructure", "@johndoe"},
				"baz.go":     {"@johndoe"},
			},
			expectApproved: true,
			expectGroups:   2,
		},
	}

	for name, test := range sharedOwnershipTests {
		t.Run(name, func(t *testing.T) {
			prctx := &pulltest.Context{
				AuthorValue:     "author",
				CommitsValue:    []*pull.Commit{{SHA: "abc123", Author: "author"}},
				HeadSHAValue:    "abc123",
				ReviewsValue:    []*pull.Review{{Author: pull.NewAuthor(test.reviewer), State: pull.ReviewApproved, CreatedAt: now}},
				CodeownersValue: &pull.CodeownersResult{Owners: test.owners},
			}
			if test.teamMemberships != nil {
				prctx.TeamMemberships = map[string][]string{test.reviewer: test.teamMemberships}
			}

			r := &Rule{
				Options:  Options{Methods: DefaultMethods()},
				Requires: Requires{Actors: common.Actors{Codeowners: true}},
			}

			candidates, _, err := r.FilteredCandidates(ctx, prctx)
			require.NoError(t, err)

			approved, result, err := r.IsApproved(ctx, prctx, candidates)
			require.NoError(t, err)
			assert.Equal(t, test.expectApproved, approved)
			assert.Len(t, result.OwnershipGroups, test.expectGroups)
		})
	}
}
