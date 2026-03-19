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
	"fmt"
	"sync"
	"time"
)

const DefaultDebounceWindow = 30 * time.Second

// StatusDebouncer coalesces rapid evaluation requests for the same pull
// request. When many check runs or status events complete in quick succession
// for the same commit, only the first triggers an immediate evaluation. A
// trailing evaluation is automatically scheduled when the debounce window
// expires, ensuring the final state is always captured after the burst.
type StatusDebouncer struct {
	mu      sync.Mutex
	entries map[string]*debounceEntry
	window  time.Duration
}

type debounceEntry struct {
	evaluatedAt time.Time
	trailTimer  *time.Timer
	trailGen    uint64
}

// NewStatusDebouncer creates a debouncer with the given window duration.
// If window is <= 0, DefaultDebounceWindow is used.
func NewStatusDebouncer(window time.Duration) *StatusDebouncer {
	if window <= 0 {
		window = DefaultDebounceWindow
	}
	return &StatusDebouncer{
		entries: make(map[string]*debounceEntry),
		window:  window,
	}
}

// Deduplicate returns true if the evaluation should proceed immediately.
// If it returns false, the evaluation is redundant within the debounce window
// and a trailing evaluation has been scheduled via trailingFn for when the
// window expires. This trailing evaluation ensures that the final state after
// a burst of events is always captured.
func (d *StatusDebouncer) Deduplicate(key string, trailingFn func()) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	entry, exists := d.entries[key]

	if !exists || now.Sub(entry.evaluatedAt) >= d.window {
		// First event or window expired — evaluate immediately
		if entry != nil && entry.trailTimer != nil {
			entry.trailTimer.Stop()
		}
		d.entries[key] = &debounceEntry{evaluatedAt: now}
		return true
	}

	// Within debounce window — skip and schedule trailing evaluation
	if entry.trailTimer != nil {
		entry.trailTimer.Stop()
	}

	entry.trailGen++
	gen := entry.trailGen
	remaining := d.window - now.Sub(entry.evaluatedAt)

	entry.trailTimer = time.AfterFunc(remaining, func() {
		d.mu.Lock()
		current, ok := d.entries[key]
		shouldRun := ok && current == entry && current.trailGen == gen
		if shouldRun {
			delete(d.entries, key)
		}
		d.mu.Unlock()

		if shouldRun {
			trailingFn()
		}
	})

	return false
}

// DebounceKey builds a deduplication key for a pull request evaluation.
// The key is per-PR only (owner/repo/number) so that different event types
// (e.g. pull_request and workflow_run) for the same PR are collapsed.
func DebounceKey(owner, repo string, number int) string {
	return fmt.Sprintf("%s/%s/%d", owner, repo, number)
}
