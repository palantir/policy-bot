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
	"math/rand"
	"sync"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy/common"
)

const (
	DefaultRateLimitRetryJitter = 5 * time.Second
	defaultSecondaryRetryAfter  = time.Minute
)

type RateLimitDeferrer struct {
	mu        sync.Mutex
	entries   map[int64]*rateLimitEntry
	maxJitter time.Duration
	jitter    func() time.Duration
	now       func() time.Time
}

type rateLimitEntry struct {
	resetAt time.Time
	timer   *time.Timer
	pending map[string]*rateLimitPending
}

type rateLimitPending struct {
	ctx     context.Context
	trigger common.Trigger
	run     func(context.Context, common.Trigger)
}

func NewRateLimitDeferrer(maxJitter time.Duration) *RateLimitDeferrer {
	if maxJitter < 0 {
		maxJitter = 0
	}
	return &RateLimitDeferrer{
		entries:   make(map[int64]*rateLimitEntry),
		maxJitter: maxJitter,
	}
}

func (d *RateLimitDeferrer) DeferIfActive(ctx context.Context, installationID int64, key string, trigger common.Trigger, run func(context.Context, common.Trigger)) bool {
	d.mu.Lock()
	entry, ok := d.entries[installationID]
	if !ok || !d.nowTime().Before(entry.resetAt) {
		d.mu.Unlock()
		return false
	}
	d.addPendingLocked(entry, ctx, key, trigger, run)
	d.mu.Unlock()
	return true
}

func (d *RateLimitDeferrer) DeferUntil(ctx context.Context, installationID int64, key string, trigger common.Trigger, resetAt time.Time, run func(context.Context, common.Trigger)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.entries[installationID]
	if !ok {
		entry = &rateLimitEntry{
			resetAt: resetAt,
			pending: make(map[string]*rateLimitPending),
		}
		d.entries[installationID] = entry
		d.scheduleLocked(installationID, entry)
	} else if resetAt.After(entry.resetAt) {
		entry.resetAt = resetAt
		if entry.timer != nil {
			entry.timer.Stop()
		}
		d.scheduleLocked(installationID, entry)
	}

	d.addPendingLocked(entry, ctx, key, trigger, run)
}

func (d *RateLimitDeferrer) addPendingLocked(entry *rateLimitEntry, ctx context.Context, key string, trigger common.Trigger, run func(context.Context, common.Trigger)) {
	pending := entry.pending[key]
	if pending == nil {
		entry.pending[key] = &rateLimitPending{
			ctx:     context.WithoutCancel(ctx),
			trigger: trigger,
			run:     run,
		}
		return
	}
	pending.ctx = context.WithoutCancel(ctx)
	pending.trigger |= trigger
	pending.run = run
}

func (d *RateLimitDeferrer) scheduleLocked(installationID int64, entry *rateLimitEntry) {
	delay := entry.resetAt.Sub(d.nowTime()) + d.jitterDelay()
	if delay < 0 {
		delay = 0
	}
	entry.timer = time.AfterFunc(delay, func() {
		d.runDeferred(installationID, entry)
	})
}

func (d *RateLimitDeferrer) runDeferred(installationID int64, entry *rateLimitEntry) {
	d.mu.Lock()
	if d.entries[installationID] != entry {
		d.mu.Unlock()
		return
	}
	delete(d.entries, installationID)

	pending := make([]*rateLimitPending, 0, len(entry.pending))
	for _, p := range entry.pending {
		pending = append(pending, p)
	}
	d.mu.Unlock()

	for _, p := range pending {
		p.run(p.ctx, p.trigger)
	}
}

func (d *RateLimitDeferrer) nowTime() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *RateLimitDeferrer) jitterDelay() time.Duration {
	if d.jitter != nil {
		return d.jitter()
	}
	if d.maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(d.maxJitter)))
}

func rateLimitResetTime(err error) (time.Time, bool) {
	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		return rateErr.Rate.Reset.Time, true
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		delay := defaultSecondaryRetryAfter
		if abuseErr.RetryAfter != nil {
			delay = *abuseErr.RetryAfter
		}
		return time.Now().Add(delay), true
	}

	return time.Time{}, false
}

func isRateLimitError(err error) bool {
	_, ok := rateLimitResetTime(err)
	return ok
}
