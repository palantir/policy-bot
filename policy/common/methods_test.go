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

package common

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestCandidates(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{
		CommentsValue: []*pull.Comment{
			{
				Body:   "I like to comment!",
				Author: "rrandom",
			},
			{
				Body:   "Looks good to me :+1:",
				Author: "mhaypenny",
			},
			{
				Body:   ":lgtm:",
				Author: "ttest",
			},
		},
		ReviewsValue: []*pull.Review{
			{
				Author: "rrandom",
				State:  pull.ReviewCommented,
			},
			{
				Author: "mhaypenny",
				State:  pull.ReviewChangesRequested,
			},
			{
				Author: "ttest",
				State:  pull.ReviewApproved,
			},
		},
	}

	t.Run("comments", func(t *testing.T) {
		m := &Methods{
			Comments: []string{":+1:", ":lgtm:"},
		}

		cs, err := m.Candidates(ctx, prctx)
		require.NoError(t, err)

		require.Len(t, cs, 2, "incorrect number of candidates found")
		assert.Equal(t, "mhaypenny", cs[0].User)
		assert.Equal(t, "ttest", cs[1].User)
	})

	t.Run("reviews", func(t *testing.T) {
		m := &Methods{
			GithubReview:      true,
			GithubReviewState: pull.ReviewChangesRequested,
		}

		cs, err := m.Candidates(ctx, prctx)
		require.NoError(t, err)

		require.Len(t, cs, 1, "incorrect number of candidates found")
		assert.Equal(t, "mhaypenny", cs[0].User)
	})
}

func TestCandidatesByLastModified(t *testing.T) {
	cs := []*Candidate{
		{
			User:         "c",
			LastModified: time.Date(2018, 6, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			User:         "a",
			LastModified: time.Date(2018, 6, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			User:         "d",
			LastModified: time.Date(2018, 6, 29, 14, 0, 0, 0, time.UTC),
		},
		{
			User:         "b",
			LastModified: time.Date(2018, 6, 29, 10, 0, 0, 0, time.UTC),
		},
	}

	sort.Sort(CandidatesByModifiedTime(cs))

	for i, u := range []string{"a", "b", "c", "d"} {
		assert.Equalf(t, u, cs[i].User, "candidate at position %d is incorrect", i)
	}
}
