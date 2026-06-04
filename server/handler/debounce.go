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
	"fmt"
	"sync"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

	// pending accumulates the union of triggers seen during the current window,
	// including the leading-edge trigger. The trailing evaluation runs with this
	// union so that a trigger the policy actually responds to (e.g. the
	// PullRequest trigger from an `opened` event) is never dropped just because
	// it arrived between two non-matching events (e.g. `labeled`).
	pending common.Trigger
	// coalesced counts the events folded into the current window, leading edge
	// included. Recorded for tracing.
	coalesced int
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

// Deduplicate returns true if the evaluation for trigger should proceed
// immediately. If it returns false, the evaluation is redundant within the
// debounce window and a trailing evaluation has been scheduled via trailingFn
// for when the window expires. This trailing evaluation ensures that the final
// state after a burst of events is always captured.
//
// Triggers are unioned across the window: trailingFn receives the OR of every
// trigger seen since the leading edge (including the leading-edge trigger),
// along with the number of coalesced events. This guarantees a policy-relevant
// trigger is never lost just because a non-matching event happened to be the
// leading edge or the last event in the burst.
//
// The decision is recorded as attributes on the span in ctx (typically the
// webhook span) so a debounced delivery is identifiable from its own trace.
func (d *StatusDebouncer) Deduplicate(ctx context.Context, key string, trigger common.Trigger, trailingFn func(ctx context.Context, accumulated common.Trigger, coalesced int)) bool {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(AttrDebounceKey, key),
		attribute.Int64(AttrDebounceWindowMs, d.window.Milliseconds()),
	)

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	entry, exists := d.entries[key]

	if !exists || now.Sub(entry.evaluatedAt) >= d.window {
		// First event or window expired — evaluate immediately
		reason := DebounceReasonFirstEvent
		if exists {
			reason = DebounceReasonWindowExpired
		}
		if entry != nil && entry.trailTimer != nil {
			entry.trailTimer.Stop()
		}
		d.entries[key] = &debounceEntry{
			evaluatedAt: now,
			pending:     trigger,
			coalesced:   1,
		}
		span.SetAttributes(
			attribute.String(AttrDebounceDecision, DebounceDecisionEvaluate),
			attribute.String(AttrDebounceReason, reason),
			attribute.String(AttrDebounceAccumulatedTrigger, trigger.String()),
		)
		return true
	}

	// Within debounce window — skip, accumulate the trigger, and (re)schedule
	// the trailing evaluation so it runs with the full union once the burst ends.
	if entry.trailTimer != nil {
		entry.trailTimer.Stop()
	}

	entry.trailGen++
	entry.pending |= trigger
	entry.coalesced++
	gen := entry.trailGen
	accumulated := entry.pending
	coalesced := entry.coalesced
	remaining := d.window - now.Sub(entry.evaluatedAt)

	span.SetAttributes(
		attribute.String(AttrDebounceDecision, DebounceDecisionSkip),
		attribute.String(AttrDebounceReason, DebounceReasonWithinWindow),
		attribute.Bool(AttrDebounceTrailingScheduled, true),
		attribute.Int64(AttrDebounceTrailingDelayMs, remaining.Milliseconds()),
		attribute.Int64(AttrDebounceTrailGen, int64(gen)),
		attribute.String(AttrDebounceAccumulatedTrigger, accumulated.String()),
		attribute.Int(AttrDebounceCoalesced, coalesced),
	)
	// Record this event's contribution so the running union is visible even
	// before the trailing evaluation fires.
	span.AddEvent("debounce.trigger_coalesced", trace.WithAttributes(
		attribute.String(AttrPolicyTrigger, trigger.String()),
		attribute.String(AttrDebounceAccumulatedTrigger, accumulated.String()),
	))

	entry.trailTimer = time.AfterFunc(remaining, func() {
		d.mu.Lock()
		current, ok := d.entries[key]
		shouldRun := ok && current == entry && current.trailGen == gen
		if shouldRun {
			delete(d.entries, key)
		}
		d.mu.Unlock()

		if shouldRun {
			trailingFn(ctx, accumulated, coalesced)
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
