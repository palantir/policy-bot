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
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				Author:       "body-editor",
			},
			CommentsValue: []*pull.Comment{
				{
					CreatedAt:    now.Add(10 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "other-user",
					Body:         "Why did you do this?",
				},
				{
					CreatedAt:    now.Add(20 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "comment-approver",
					Body:         "LGTM :+1: :shipit:",
				},
				{
					CreatedAt:    now.Add(30 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "disapprover",
					Body:         "I don't like things! :-1:",
				},
				{
					CreatedAt:    now.Add(40 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "mhaypenny",
					Body:         ":+1: my stuff is cool",
				},
				{
					CreatedAt:    now.Add(50 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "contributor-author",
					Body:         ":+1: I added to this PR",
				},
				{
					CreatedAt:    now.Add(60 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "contributor-committer",
					Body:         ":+1: I also added to this PR",
				},
				{
					CreatedAt:    now.Add(70 * time.Second),
					LastEditedAt: now.Add(71 * time.Second),
					Author:       "comment-editor",
					Body:         "LGTM :+1: :shipit:",
				},
			},
			ReviewsValue: []*pull.Review{
				{
					CreatedAt:    now.Add(70 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "disapprover",
					State:        pull.ReviewChangesRequested,
					Body:         "I _really_ don't like things!",
				},
				{
					CreatedAt:    now.Add(80 * time.Second),
					LastEditedAt: time.Time{},
					Author:       "review-approver",
					State:        pull.ReviewApproved,
					Body:         "I LIKE THIS",
				},
				{
					CreatedAt:    now.Add(90 * time.Second),
					LastEditedAt: now.Add(91 * time.Second),
					Author:       "review-comment-editor",
					State:        pull.ReviewApproved,
					Body:         "I LIKE THIS",
				},
			},
			CommitsValue: []*pull.Commit{
				{
					PushedAt:  newTime(now.Add(5 * time.Second)),
					SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
					Author:    "mhaypenny",
					Committer: "mhaypenny",
				},
				{
					PushedAt:  newTime(now.Add(15 * time.Second)),
					SHA:       "674832587eaaf416371b30f5bc5a47e377f534ec",
					Author:    "contributor-author",
					Committer: "mhaypenny",
				},
				{
					PushedAt:  newTime(now.Add(45 * time.Second)),
					SHA:       "97d5ea26da319a987d80f6db0b7ef759f2f2e441",
					Author:    "mhaypenny",
					Committer: "contributor-committer",
				},
			},
			OrgMemberships: map[string][]string{
				"mhaypenny":             {"everyone"},
				"contributor-author":    {"everyone"},
				"contributor-committer": {"everyone"},
				"comment-approver":      {"everyone", "cool-org"},
				"review-approver":       {"everyone", "even-cooler-org"},
			},
		}
	}

	assertApproved := func(t *testing.T, prctx pull.Context, r *Rule, expected string) {
		allowedCandidates, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, msg, err := r.IsApproved(ctx, prctx, allowedCandidates)
		require.NoError(t, err)

		if assert.True(t, approved, "pull request was not approved") {
			assert.Equal(t, expected, msg)
		}
	}

	assertPending := func(t *testing.T, prctx pull.Context, r *Rule, expected string) {
		allowedCandidates, err := r.FilteredCandidates(ctx, prctx)
		require.NoError(t, err)

		approved, msg, err := r.IsApproved(ctx, prctx, allowedCandidates)
		require.NoError(t, err)

		if assert.False(t, approved, "pull request was incorrectly approved") {
			assert.Equal(t, expected, msg)
		}
	}

	t.Run("noApprovalRequired", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{}
		assertApproved(t, prctx, r, "No approval required")
	})

	t.Run("singleApprovalRequired", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Requires: Requires{
				Count: 1,
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 7 approvals from disqualified users")
	})

	t.Run("authorCannotApprove", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Options: Options{
				AllowAuthor: false,
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
				AllowAuthor: true,
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
				AllowContributor: false,
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
				AllowContributor: true,
				AllowAuthor:      false,
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
				AllowNonAuthorContributor: true,
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
				AllowNonAuthorContributor: true,
				AllowAuthor:               true,
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
				AllowNonAuthorContributor: true,
				AllowContributor:          true,
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
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r = &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"does-not-exist"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 7 approvals from disqualified users")
	})

	t.Run("specificOrgApproves", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"cool-org"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r = &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"does-not-exist", "other-org"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 7 approvals from disqualified users")
	})

	t.Run("specificOrgsOrUserApproves", func(t *testing.T) {
		prctx := basePullContext()
		r := &Rule{
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
		prctx.CommitsValue = []*pull.Commit{
			{
				PushedAt:  newTime(now.Add(25 * time.Second)),
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r.Options.InvalidateOnPush = true
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 6 approvals from disqualified users")
	})

	t.Run("invalidateReviewOnPush", func(t *testing.T) {
		prctx := basePullContext()
		prctx.CommitsValue = []*pull.Commit{
			{
				PushedAt:  newTime(now.Add(85 * time.Second)),
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"review-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by review-approver")

		r.Options.InvalidateOnPush = true
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 1 approval from disqualified users")
	})

	t.Run("ignoreUpdateMergeAfterReview", func(t *testing.T) {
		prctx := basePullContext()
		prctx.CommitsValue = append(prctx.CommitsValue[:1], &pull.Commit{
			PushedAt:        newTime(now.Add(25 * time.Second)),
			SHA:             "647c5078288f0ea9de27b5c280f25edaf2089045",
			CommittedViaWeb: true,
			Parents: []string{
				"c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				"2e1b0bb6ab144bf7a1b7a1df9d3bdcb0fe85a206",
			},
			Author: "merge-committer",
		})

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
			Options: Options{
				InvalidateOnPush: true,
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 6 approvals from disqualified users")

		r.Options.IgnoreUpdateMerges = true
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreUpdateMergeContributor", func(t *testing.T) {
		prctx := basePullContext()
		prctx.CommitsValue = append(prctx.CommitsValue[:1], &pull.Commit{
			PushedAt:        newTime(now.Add(25 * time.Second)),
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
			Author:    "merge-committer",
			Body:      ":+1:",
		})

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"merge-committer"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 8 approvals from disqualified users")

		r.Options.IgnoreUpdateMerges = true
		assertApproved(t, prctx, r, "Approved by merge-committer")
	})

	t.Run("ignoreCommits", func(t *testing.T) {
		prctx := basePullContext()
		prctx.CommitsValue = append(prctx.CommitsValue, &pull.Commit{
			SHA:       "ea9be5fcd016dc41d70dc457dfee2e64a8f951c1",
			Author:    "comment-approver",
			Committer: "comment-approver",
		})

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 7 approvals from disqualified users")

		r.Options.IgnoreCommitsBy = common.Actors{
			Users: []string{"comment-approver"},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreCommitsMixedAuthorCommiter", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"contributor-author"},
				},
			},
			Options: Options{
				IgnoreCommitsBy: common.Actors{
					Users: []string{"contributor-author"},
				},
			},
		}
		assertPending(t, prctx, r, "0/1 approvals required. Ignored 7 approvals from disqualified users")
	})

	t.Run("ignoreCommitsInvalidateOnPush", func(t *testing.T) {
		prctx := basePullContext()
		prctx.CommitsValue = []*pull.Commit{
			{
				PushedAt:  newTime(now.Add(25 * time.Second)),
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
		}

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")

		r.Options.InvalidateOnPush = true
		r.Options.IgnoreCommitsBy = common.Actors{
			Users: []string{"mhaypenny"},
		}
		assertApproved(t, prctx, r, "Approved by comment-approver")
	})

	t.Run("ignoreEditedReviewComments", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"review-comment-editor"},
				},
			},
		}

		assertApproved(t, prctx, r, "Approved by review-comment-editor")

		r.Options.IgnoreEditedComments = true

		assertPending(t, prctx, r, "0/1 approvals required. Ignored 5 approvals from disqualified users")
	})

	t.Run("ignoreEditedComments", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-editor"},
				},
			},
		}

		assertApproved(t, prctx, r, "Approved by comment-editor")

		r.Options.IgnoreEditedComments = true

		assertPending(t, prctx, r, "0/1 approvals required. Ignored 5 approvals from disqualified users")
	})

	t.Run("ignoreEditedCommentsWithBodyPattern", func(t *testing.T) {
		prctx := basePullContext()

		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"body-editor"},
				},
			},
			Options: Options{
				Methods: &common.Methods{
					BodyPatterns: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("/no-platform")),
					},
				},
				IgnoreEditedComments: false,
			},
		}

		assertApproved(t, prctx, r, "Approved by body-editor")

		r.Options.IgnoreEditedComments = true

		assertPending(t, prctx, r, "0/1 approvals required. Ignored 5 approvals from disqualified users")
	})
}

func TestTrigger(t *testing.T) {
	t.Run("triggerCommitOnRules", func(t *testing.T) {
		r := &Rule{}

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
				IgnoreEditedComments: false,
			},
			Requires: Requires{
				Count: 1,
			},
		}

		assert.True(t, r.Trigger().Matches(common.TriggerPullRequest), "expected %s to match %s", r.Trigger(), common.TriggerPullRequest)
	})
}

func newTime(t time.Time) *time.Time {
	return &t
}
