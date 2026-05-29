// Copyright 2024 Palantir Technologies, Inc.
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopTrail is a trailing function that ignores its arguments, for tests that
// only assert on the immediate deduplication decision.
func noopTrail(context.Context, common.Trigger, int) {}

func TestStatusDebouncer_FirstEventEvaluatesImmediately(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)
	result := d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail)
	assert.True(t, result, "first event should evaluate immediately")
}

func TestStatusDebouncer_SubsequentEventsAreSkipped(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)

	result1 := d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail)
	assert.True(t, result1, "first event should evaluate")

	result2 := d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail)
	assert.False(t, result2, "second event within window should be skipped")

	result3 := d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail)
	assert.False(t, result3, "third event within window should be skipped")
}

func TestStatusDebouncer_DifferentKeysAreIndependent(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)

	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail))
	assert.True(t, d.Deduplicate(context.Background(), "key2", common.TriggerCommit, noopTrail))
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail))
	assert.False(t, d.Deduplicate(context.Background(), "key2", common.TriggerCommit, noopTrail))
}

func TestStatusDebouncer_WindowExpiry(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail))
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail))

	time.Sleep(60 * time.Millisecond)

	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail), "should evaluate after window expires")
}

func TestStatusDebouncer_TrailingEvaluation(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	var trailingCalled atomic.Int32

	// First event evaluates immediately
	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, func(context.Context, common.Trigger, int) {
		trailingCalled.Add(1)
	}))

	// Second event is skipped but schedules a trailing eval
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, func(context.Context, common.Trigger, int) {
		trailingCalled.Add(1)
	}))

	// Wait for the trailing evaluation to fire
	time.Sleep(80 * time.Millisecond)

	assert.Equal(t, int32(1), trailingCalled.Load(), "trailing evaluation should have run exactly once")
}

func TestStatusDebouncer_TrailingEvalUsesLatestFn(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	var firstTrailCalled atomic.Bool
	var secondTrailCalled atomic.Bool

	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, noopTrail))

	// First skipped event
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, func(context.Context, common.Trigger, int) {
		firstTrailCalled.Store(true)
	}))

	// Second skipped event — should supersede the first trailing fn
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerCommit, func(context.Context, common.Trigger, int) {
		secondTrailCalled.Store(true)
	}))

	time.Sleep(80 * time.Millisecond)

	assert.False(t, firstTrailCalled.Load(), "first trailing fn should not have run")
	assert.True(t, secondTrailCalled.Load(), "second (latest) trailing fn should have run")
}

// TestStatusDebouncer_TrailingUnionsTriggers covers a burst whose leading edge
// and final event are both non-matching (Label), with a policy-relevant trigger
// (PullRequest) sandwiched in the middle. The trailing evaluation must receive
// the union of all triggers so the PullRequest trigger is not dropped.
func TestStatusDebouncer_TrailingUnionsTriggers(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	var (
		mu           sync.Mutex
		gotTrigger   common.Trigger
		gotCoalesced int
	)
	record := func(_ context.Context, accumulated common.Trigger, coalesced int) {
		mu.Lock()
		defer mu.Unlock()
		gotTrigger = accumulated
		gotCoalesced = coalesced
	}

	// Leading edge: Label (does not match a PullRequest-only policy)
	assert.True(t, d.Deduplicate(context.Background(), "key1", common.TriggerLabel, record))
	// Middle: the PullRequest trigger that the policy actually responds to
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerPullRequest, record))
	// Final: Label again
	assert.False(t, d.Deduplicate(context.Background(), "key1", common.TriggerLabel, record))

	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, gotTrigger.Matches(common.TriggerPullRequest), "trailing eval must include the PullRequest trigger from the middle event")
	assert.Equal(t, common.TriggerLabel|common.TriggerPullRequest, gotTrigger, "trailing eval must receive the union of all triggers in the burst")
	assert.Equal(t, 3, gotCoalesced, "all three events should be counted as coalesced")
}

func TestStatusDebouncer_DefaultWindow(t *testing.T) {
	d := NewStatusDebouncer(0)
	require.Equal(t, DefaultDebounceWindow, d.window)
}

func TestDebounceKey(t *testing.T) {
	key := DebounceKey("owner", "repo", 42)
	assert.Equal(t, "owner/repo/42", key)
}
