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

package disapproval

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestIsDisapproved(t *testing.T) {
	logger := zerolog.New(os.Stdout)
	ctx := logger.WithContext(context.Background())

	prctx := &pulltest.Context{
		CommentsValue: []*pull.Comment{
			{
				Author:    "disapprover-1",
				Body:      "me no like :-1:",
				CreatedAt: date(0),
			},
			{
				Author:    "disapprover-1",
				Body:      "nah, is fine :+1:",
				CreatedAt: date(1),
			},
			{
				Author:    "disapprover-2",
				Body:      "me also no like :-1:",
				CreatedAt: date(2),
			},
			{
				Author:    "disapprover-3",
				Body:      "and me :-1:",
				CreatedAt: date(3),
			},
			{
				Author:    "revoker-1",
				Body:      "you all wrong :+1:",
				CreatedAt: date(4),
			},
		},
		ReviewsValue: []*pull.Review{
			{
				Author:    "disapprover-4",
				State:     pull.ReviewChangesRequested,
				CreatedAt: date(5),
			},
			{
				Author:    "revoker-2",
				State:     pull.ReviewApproved,
				CreatedAt: date(6),
			},
		},
	}

	assertDisapproved := func(t *testing.T, p *Policy, expected string) {
		disapproved, msg, err := p.IsDisapproved(ctx, prctx)
		require.NoError(t, err)

		if assert.True(t, disapproved, "pull request was not disapproved") {
			assert.Equal(t, expected, msg)
		}
	}

	assertSkipped := func(t *testing.T, p *Policy, expected string) {
		disapproved, msg, err := p.IsDisapproved(ctx, prctx)
		require.NoError(t, err)

		if assert.False(t, disapproved, "pull request was incorrectly disapproved") {
			assert.Equal(t, expected, msg)
		}
	}

	t.Run("skippedWithNoRequires", func(t *testing.T) {
		p := &Policy{}
		assertSkipped(t, p, "No disapprovals")
	})

	t.Run("singleUserDisapproves", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-2"}

		assertDisapproved(t, p, "Disapproved by disapprover-2")
	})

	t.Run("singleUserDisapprovesAndRevokes", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-1"}

		assertSkipped(t, p, "Disapproval revoked by disapprover-1")
	})

	t.Run("multipleUsersDisapprove", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-2", "disapprover-3"}

		assertDisapproved(t, p, "Disapproved by disapprover-3")
	})

	t.Run("otherUserRevokes", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-2", "disapprover-3", "revoker-1"}

		assertSkipped(t, p, "Disapproval revoked by revoker-1")
	})

	t.Run("singleUserDisapprovesWithReview", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-4"}

		assertDisapproved(t, p, "Disapproved by disapprover-4")
	})

	t.Run("otherUserRevokesWithReview", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-4", "revoker-2"}

		assertSkipped(t, p, "Disapproval revoked by revoker-2")
	})

	t.Run("reviewsInteractWithComments", func(t *testing.T) {
		p := &Policy{}
		p.Requires.Users = []string{"disapprover-1", "revoker-1", "disapprover-4"}

		assertDisapproved(t, p, "Disapproved by disapprover-4")
	})
}

func date(hour int) time.Time {
	return time.Date(2018, 6, 29, hour, 0, 0, 0, time.UTC)
}
