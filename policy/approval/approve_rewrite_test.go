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

package approval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterInvalidCandidatesHeadRewrites(t *testing.T) {
	const (
		shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	t0 := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)
	t3 := t0.Add(3 * time.Hour)
	t4 := t0.Add(4 * time.Hour)
	evalTime := t0.Add(5 * time.Hour)

	newRule := func(opts Options) *Rule {
		opts.InvalidateOnPush = new(true)
		opts.Methods = DefaultMethods()
		return &Rule{
			Options: opts,
			Requires: Requires{
				Count: 1,
				Actors: common.Actors{
					Users: []string{"maintainer"},
				},
			},
		}
	}

	review := func(sha string, at time.Time) *pull.Review {
		return &pull.Review{
			ID:        "review-" + sha[:1],
			SHA:       sha,
			CreatedAt: at,
			Author:    "maintainer",
			State:     pull.ReviewApproved,
		}
	}

	comment := func(at time.Time) *pull.Comment {
		return &pull.Comment{
			CreatedAt: at,
			Author:    "maintainer",
			Body:      ":+1:",
		}
	}

	// A is the only commit. It was first pushed at t0, replaced by B, and
	// force-pushed back at t3.
	replayedHead := func() *pulltest.Context {
		return &pulltest.Context{
			EvaluationTimestampValue: evalTime,
			HeadSHAValue:             shaA,
			CommitsValue: []*pull.Commit{
				{SHA: shaA, Author: "contributor", Committer: "contributor"},
			},
			PushedAtValue: map[string]time.Time{
				shaA: t0,
			},
			LatestHeadRewriteValue: &pull.HeadRewrite{At: t3, SHA: shaA},
		}
	}

	// A is a child of B. A was pushed at t0, the head was force-pushed to B
	// at t1, and A was later pushed again as a fast-forward.
	fastForwardedHead := func() *pulltest.Context {
		return &pulltest.Context{
			EvaluationTimestampValue: evalTime,
			HeadSHAValue:             shaA,
			CommitsValue: []*pull.Commit{
				{SHA: shaB, Author: "contributor", Committer: "contributor"},
				{SHA: shaA, Author: "contributor", Committer: "contributor", Parents: []string{shaB}},
			},
			PushedAtValue: map[string]time.Time{
				shaA: t0,
				shaB: t1,
			},
			LatestHeadRewriteValue: &pull.HeadRewrite{At: t1, SHA: shaB},
		}
	}

	filter := func(t *testing.T, r *Rule, prctx pull.Context) ([]*common.Candidate, []*common.Dismissal) {
		candidates, dismissals, err := r.FilteredCandidates(context.Background(), prctx)
		require.NoError(t, err)
		return candidates, dismissals
	}

	t.Run("reviewOfReplacedCommit", func(t *testing.T) {
		prctx := replayedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaB, t2)}

		candidates, dismissals := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
		require.Len(t, dismissals, 1)
		assert.Equal(t, "Review was not for the current head (aaaaaaa)", dismissals[0].Reason)
	})

	t.Run("reviewOfReplacedCommitWithoutRewriteEvent", func(t *testing.T) {
		prctx := replayedHead()
		prctx.LatestHeadRewriteValue = nil
		prctx.ReviewsValue = []*pull.Review{review(shaB, t2)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
	})

	t.Run("reviewWithoutSHA", func(t *testing.T) {
		prctx := replayedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaA, t4)}
		prctx.ReviewsValue[0].SHA = ""

		candidates, _ := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
	})

	t.Run("reviewOfHeadBeforeRewrite", func(t *testing.T) {
		prctx := replayedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaA, t2)}

		candidates, dismissals := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
		require.Len(t, dismissals, 1)
		assert.Equal(t, "Invalidated by force-push or other rewrite at 2026-09-28T15:00:00Z", dismissals[0].Reason)
	})

	t.Run("reviewOfHeadAfterRewrite", func(t *testing.T) {
		prctx := replayedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaA, t4)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		require.Len(t, candidates, 1)
	})

	t.Run("reviewOfHeadBeforeReopen", func(t *testing.T) {
		prctx := replayedHead()
		prctx.LatestHeadRewriteValue = &pull.HeadRewrite{At: t3}
		prctx.ReviewsValue = []*pull.Review{review(shaA, t2)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
	})

	t.Run("commentBeforeForcePushToReplayedHead", func(t *testing.T) {
		prctx := replayedHead()
		prctx.CommentsValue = []*pull.Comment{comment(t2)}

		candidates, dismissals := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
		require.Len(t, dismissals, 1)
		assert.Equal(t, "Invalidated by force-push or other rewrite at 2026-09-28T15:00:00Z", dismissals[0].Reason)
	})

	t.Run("commentAfterForcePushToReplayedHead", func(t *testing.T) {
		prctx := replayedHead()
		prctx.CommentsValue = []*pull.Comment{comment(t4)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		require.Len(t, candidates, 1)
	})

	t.Run("commentAfterForcePushBeforeFastForward", func(t *testing.T) {
		prctx := fastForwardedHead()
		prctx.CommentsValue = []*pull.Comment{comment(t2)}

		candidates, dismissals := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
		require.Len(t, dismissals, 1)
		assert.Equal(t, "Invalidated because aaaaaaa was pushed again after a force-push", dismissals[0].Reason)
	})

	t.Run("reviewAfterFastForward", func(t *testing.T) {
		prctx := fastForwardedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaA, t4)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		require.Len(t, candidates, 1)
	})

	t.Run("reviewOfParentBeforeFastForward", func(t *testing.T) {
		prctx := fastForwardedHead()
		prctx.ReviewsValue = []*pull.Review{review(shaB, t2)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		assert.Empty(t, candidates)
	})

	t.Run("commentAfterForcePushDroppingCommits", func(t *testing.T) {
		prctx := fastForwardedHead()
		prctx.HeadSHAValue = shaB
		prctx.CommitsValue = prctx.CommitsValue[:1]
		prctx.PushedAtValue = map[string]time.Time{shaB: t0}
		prctx.CommentsValue = []*pull.Comment{comment(t2)}

		candidates, _ := filter(t, newRule(Options{}), prctx)
		require.Len(t, candidates, 1)
	})

	t.Run("reviewOfLastCommitBelowUpdateMerge", func(t *testing.T) {
		const shaMerge = "cccccccccccccccccccccccccccccccccccccccc"

		prctx := replayedHead()
		prctx.LatestHeadRewriteValue = nil
		prctx.HeadSHAValue = shaMerge
		prctx.CommitsValue = append(prctx.CommitsValue, &pull.Commit{
			SHA:             shaMerge,
			CommittedViaWeb: true,
			Parents:         []string{shaA, "dddddddddddddddddddddddddddddddddddddddd"},
			Author:          "merge-committer",
		})
		prctx.ReviewsValue = []*pull.Review{review(shaA, t2)}

		candidates, _ := filter(t, newRule(Options{IgnoreUpdateMerges: new(true)}), prctx)
		require.Len(t, candidates, 1)
	})

	t.Run("rewriteError", func(t *testing.T) {
		prctx := replayedHead()
		prctx.LatestHeadRewriteValue = nil
		prctx.LatestHeadRewriteError = errors.New("timeline unavailable")
		prctx.ReviewsValue = []*pull.Review{review(shaA, t4)}

		_, _, err := newRule(Options{}).FilteredCandidates(context.Background(), prctx)
		assert.Error(t, err)
	})
}
