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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusDebouncer_FirstEventEvaluatesImmediately(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)
	result := d.Deduplicate("key1", func() {})
	assert.True(t, result, "first event should evaluate immediately")
}

func TestStatusDebouncer_SubsequentEventsAreSkipped(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)

	result1 := d.Deduplicate("key1", func() {})
	assert.True(t, result1, "first event should evaluate")

	result2 := d.Deduplicate("key1", func() {})
	assert.False(t, result2, "second event within window should be skipped")

	result3 := d.Deduplicate("key1", func() {})
	assert.False(t, result3, "third event within window should be skipped")
}

func TestStatusDebouncer_DifferentKeysAreIndependent(t *testing.T) {
	d := NewStatusDebouncer(100 * time.Millisecond)

	assert.True(t, d.Deduplicate("key1", func() {}))
	assert.True(t, d.Deduplicate("key2", func() {}))
	assert.False(t, d.Deduplicate("key1", func() {}))
	assert.False(t, d.Deduplicate("key2", func() {}))
}

func TestStatusDebouncer_WindowExpiry(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	assert.True(t, d.Deduplicate("key1", func() {}))
	assert.False(t, d.Deduplicate("key1", func() {}))

	time.Sleep(60 * time.Millisecond)

	assert.True(t, d.Deduplicate("key1", func() {}), "should evaluate after window expires")
}

func TestStatusDebouncer_TrailingEvaluation(t *testing.T) {
	d := NewStatusDebouncer(50 * time.Millisecond)

	var trailingCalled atomic.Int32

	// First event evaluates immediately
	assert.True(t, d.Deduplicate("key1", func() {
		trailingCalled.Add(1)
	}))

	// Second event is skipped but schedules a trailing eval
	assert.False(t, d.Deduplicate("key1", func() {
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

	assert.True(t, d.Deduplicate("key1", func() {}))

	// First skipped event
	assert.False(t, d.Deduplicate("key1", func() {
		firstTrailCalled.Store(true)
	}))

	// Second skipped event — should supersede the first trailing fn
	assert.False(t, d.Deduplicate("key1", func() {
		secondTrailCalled.Store(true)
	}))

	time.Sleep(80 * time.Millisecond)

	assert.False(t, firstTrailCalled.Load(), "first trailing fn should not have run")
	assert.True(t, secondTrailCalled.Load(), "second (latest) trailing fn should have run")
}

func TestStatusDebouncer_DefaultWindow(t *testing.T) {
	d := NewStatusDebouncer(0)
	require.Equal(t, DefaultDebounceWindow, d.window)
}

func TestDebounceKey(t *testing.T) {
	key := DebounceKey("owner", "repo", 42, "status")
	assert.Equal(t, "owner/repo/42/status", key)
}
