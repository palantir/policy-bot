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
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCoalescer closes windows only when the test says so, and records every evaluation that ran
type testCoalescer struct {
	*Coalescer
	closers []func()
	ran     []*evaluation
	errs    []error
}

func newTestCoalescer() *testCoalescer {
	tc := &testCoalescer{Coalescer: NewCoalescer(time.Minute)}
	tc.after = func(_ time.Duration, f func()) *time.Timer {
		tc.closers = append(tc.closers, f)
		return nil
	}
	return tc
}

func (tc *testCoalescer) run(_ context.Context, e *evaluation) error {
	tc.ran = append(tc.ran, e)
	if len(tc.errs) == 0 {
		return nil
	}
	err := tc.errs[0]
	tc.errs = tc.errs[1:]
	return err
}

// closeWindows closes every window opened so far
func (tc *testCoalescer) closeWindows() {
	closers := tc.closers
	tc.closers = nil
	for _, f := range closers {
		f()
	}
}

func (tc *testCoalescer) evaluate(number int, trigger common.Trigger, runs ...*github.WorkflowRun) error {
	loc := pull.Locator{Owner: "org", Repo: "repo", Number: number}
	e := &evaluation{installationID: 1, trigger: trigger, loc: loc, runs: runs, run: tc.run, events: 1}
	return tc.Evaluate(context.Background(), e)
}

func TestCoalescerRunsFirstThenHoldsTheRest(t *testing.T) {
	tc := newTestCoalescer()
	run := &github.WorkflowRun{ID: new(int64(7))}

	require.NoError(t, tc.evaluate(1, common.TriggerCommit))
	require.NoError(t, tc.evaluate(1, common.TriggerStatus, run))
	require.NoError(t, tc.evaluate(1, common.TriggerStatus))
	require.NoError(t, tc.evaluate(2, common.TriggerCommit))
	require.Len(t, tc.ran, 2, "the first evaluation of each pull request runs at once")

	tc.closeWindows()
	require.Len(t, tc.ran, 3, "the held evaluations of pull request 1 run as one")
	held := tc.ran[2]
	assert.Equal(t, 1, held.loc.Number)
	assert.Equal(t, common.TriggerStatus, held.trigger)
	assert.Equal(t, []*github.WorkflowRun{run}, held.runs)
	assert.Equal(t, 2, held.events)

	tc.closeWindows()
	require.Len(t, tc.ran, 3, "an empty window runs nothing")
	require.NoError(t, tc.evaluate(1, common.TriggerCommit))
	require.Len(t, tc.ran, 4, "a pull request whose windows have closed is evaluated at once again")
}

func TestCoalescerReturnsTheFirstEvaluationsError(t *testing.T) {
	tc := newTestCoalescer()
	tc.errs = []error{errors.New("boom")}

	assert.EqualError(t, tc.evaluate(1, common.TriggerCommit), "boom")
	assert.NoError(t, tc.evaluate(1, common.TriggerCommit), "a held evaluation has no error to return yet")
}

func TestCoalescerRetriesHeldEvaluationOnTransientError(t *testing.T) {
	tc := newTestCoalescer()
	require.NoError(t, tc.evaluate(1, common.TriggerCommit))
	require.NoError(t, tc.evaluate(1, common.TriggerStatus))

	tc.errs = []error{statusError(502), errors.New("bad policy")}
	tc.closeWindows()
	require.Len(t, tc.ran, 2)
	tc.closeWindows()
	require.Len(t, tc.ran, 3, "the held evaluation runs again in the next window after a transient error")
	tc.closeWindows()
	require.Len(t, tc.ran, 3, "a held evaluation that fails for good is dropped")
}

func TestCoalescerGivesUpOnTransientErrors(t *testing.T) {
	tc := newTestCoalescer()
	require.NoError(t, tc.evaluate(1, common.TriggerCommit))
	require.NoError(t, tc.evaluate(1, common.TriggerStatus))

	for range coalesceAttempts + 2 {
		tc.errs = []error{statusError(502)}
		tc.closeWindows()
	}
	assert.Len(t, tc.ran, 1+coalesceAttempts)
}

func TestEvaluationAbsorbKeepsTheCurrentPullRequest(t *testing.T) {
	full := func(sha string) *github.PullRequest {
		return &github.PullRequest{
			Draft:     new(false),
			Title:     new("title"),
			CreatedAt: &github.Timestamp{Time: time.Now()},
			State:     new("open"),
			User:      &github.User{Login: new("author")},
			Base:      &github.PullRequestBranch{Ref: new("main"), Repo: &github.Repository{ID: new(int64(1))}},
			Head: &github.PullRequestBranch{
				Ref:  new("feature"),
				SHA:  new(sha),
				Repo: &github.Repository{ID: new(int64(1)), Name: new("repo"), Owner: &github.User{Login: new("org")}},
			},
			ChangedFiles: new(1),
		}
	}
	partial := func(sha string) *github.PullRequest {
		return &github.PullRequest{Head: &github.PullRequestBranch{SHA: new(sha)}}
	}
	locate := func(pr *github.PullRequest) pull.Locator {
		return pull.Locator{Owner: "org", Repo: "repo", Number: 1, Value: pr}
	}

	tests := map[string]struct {
		First, Second *github.PullRequest
		KeepsSecond   bool
	}{
		"fullAfterFull":            {full("a"), full("b"), true},
		"partialAfterFullSameHead": {full("a"), partial("a"), false},
		"partialAfterFullNewHead":  {full("a"), partial("b"), true},
		"fullAfterPartial":         {partial("a"), full("a"), true},
		"partialAfterPartial":      {partial("a"), partial("a"), true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			e := &evaluation{loc: locate(test.First), trigger: common.TriggerCommit, events: 1}
			e.absorb(&evaluation{loc: locate(test.Second), trigger: common.TriggerStatus, events: 1})
			expected := test.First
			if test.KeepsSecond {
				expected = test.Second
			}
			assert.Same(t, expected, e.loc.Value)
			assert.Equal(t, common.TriggerCommit|common.TriggerStatus, e.trigger)
			assert.Equal(t, 2, e.events)
		})
	}
}
