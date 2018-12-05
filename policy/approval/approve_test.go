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
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestIsApproved(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	ctx := logger.WithContext(context.Background())

	now := time.Now()
	prctx := &pulltest.Context{
		AuthorValue: "mhaypenny",
		CommentsValue: []*pull.Comment{
			{
				Order:        10,
				Author:       "other-user",
				Body:         "Why did you do this?",
				LastModified: now.Add(1 * time.Second),
			},
			{
				Order:        20,
				Author:       "comment-approver",
				Body:         "LGTM :+1: :shipit:",
				LastModified: now.Add(2 * time.Second),
			},
			{
				Order:        30,
				Author:       "disapprover",
				Body:         "I don't like things! :-1:",
				LastModified: now.Add(3 * time.Second),
			},
			{
				Order:        40,
				Author:       "mhaypenny",
				Body:         ":+1: my stuff is cool",
				LastModified: now.Add(4 * time.Second),
			},
			{
				Order:        50,
				Author:       "contributor-author",
				Body:         ":+1: I added to this PR",
				LastModified: now.Add(5 * time.Second),
			},
			{
				Order:        60,
				Author:       "contributor-committer",
				Body:         ":+1: I also added to this PR",
				LastModified: now.Add(6 * time.Second),
			},
		},
		ReviewsValue: []*pull.Review{
			{
				Order:        70,
				Author:       "disapprover",
				State:        pull.ReviewChangesRequested,
				Body:         "I _really_ don't like things!",
				LastModified: now.Add(7 * time.Second),
			},
			{
				Order:        80,
				Author:       "review-approver",
				State:        pull.ReviewApproved,
				Body:         "I LIKE THIS",
				LastModified: now.Add(8 * time.Second),
			},
		},
		CommitsValue: []*pull.Commit{
			{
				Order:     90,
				SHA:       "c6ade256ecfc755d8bc877ef22cc9e01745d46bb",
				Author:    "mhaypenny",
				Committer: "mhaypenny",
			},
			{
				Order:     100,
				SHA:       "674832587eaaf416371b30f5bc5a47e377f534ec",
				Author:    "contributor-author",
				Committer: "mhaypenny",
			},
			{
				Order:     110,
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

	assertApproved := func(t *testing.T, r *Rule, expected string) {
		approved, msg, err := r.IsApproved(ctx, prctx)
		require.NoError(t, err)

		if assert.True(t, approved, "pull request was not approved") {
			assert.Equal(t, expected, msg)
		}
	}

	assertPending := func(t *testing.T, r *Rule, expected string) {
		approved, msg, err := r.IsApproved(ctx, prctx)
		require.NoError(t, err)

		if assert.False(t, approved, "pull request was incorrectly approved") {
			assert.Equal(t, expected, msg)
		}
	}

	t.Run("noApprovalRequired", func(t *testing.T) {
		r := &Rule{}
		assertApproved(t, r, "No approval required")
	})

	t.Run("singleApprovalRequired", func(t *testing.T) {
		r := &Rule{
			Requires: Requires{
				Count: 1,
			},
		}
		assertPending(t, r, "1 approval needed. Ignored 5 approvals from disqualified users")
	})

	t.Run("authorCannotApprove", func(t *testing.T) {
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
		assertApproved(t, r, "Approved by comment-approver, review-approver")
	})

	t.Run("contributorsCannotApprove", func(t *testing.T) {
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
		assertApproved(t, r, "Approved by comment-approver, review-approver")
	})

	t.Run("contributorsCanApprove", func(t *testing.T) {
		r := &Rule{
			Options: Options{
				AllowContributor: true,
			},
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"everyone"},
				},
			},
		}
		assertApproved(t, r, "Approved by comment-approver, mhaypenny, contributor-author, contributor-committer, review-approver")
	})

	t.Run("specificUserApproves", func(t *testing.T) {
		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"comment-approver"},
				},
			},
		}
		assertApproved(t, r, "Approved by comment-approver")

		r = &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"does-not-exist"},
				},
			},
		}
		assertPending(t, r, "1 approval needed. Ignored 5 approvals from disqualified users")
	})

	t.Run("specificOrgApproves", func(t *testing.T) {
		r := &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"cool-org"},
				},
			},
		}
		assertApproved(t, r, "Approved by comment-approver")

		r = &Rule{
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Organizations: []string{"does-not-exist", "other-org"},
				},
			},
		}
		assertPending(t, r, "1 approval needed. Ignored 5 approvals from disqualified users")
	})

	t.Run("specificOrgsOrUserApproves", func(t *testing.T) {
		r := &Rule{
			Requires: Requires{
				Count: 2,
				Actors: common.Actors{
					Users:         []string{"review-approver"},
					Organizations: []string{"cool-org"},
				},
			},
		}
		assertApproved(t, r, "Approved by comment-approver, review-approver")
	})

	t.Run("invalidateOnPush_comment", func(t *testing.T) {
		prctx.CommitsValue = []*pull.Commit{
			{
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
			Options: Options{
				InvalidateOnPush: true,
			},
		}

		// set the commit after the comment
		prctx.CommitsValue[0].Order = 25
		assertPending(t, r, "1 approval needed. Ignored 4 approvals from disqualified users")

		// set the commit before the comment
		prctx.CommitsValue[0].Order = 15
		assertApproved(t, r, "Approved by comment-approver")
	})

	t.Run("invalidateOnPush_review", func(t *testing.T) {
		prctx.CommitsValue = []*pull.Commit{
			{
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
			Options: Options{
				InvalidateOnPush: true,
			},
		}

		// set the commit after the review
		prctx.CommitsValue[0].Order = 85
		assertPending(t, r, "1 approval needed (of required 1)")

		// set the commit before the review
		prctx.CommitsValue[0].Order = 75
		assertApproved(t, r, "Approved by review-approver")
	})
}
