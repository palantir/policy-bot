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
	"fmt"
	"sync"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/rs/zerolog"
)

// DefaultCoalesceWindow is how long evaluations of a pull request are held after one runs.
// A push starts every workflow at once and each one's completion is an event, so one more evaluation covers them all.
const DefaultCoalesceWindow = 30 * time.Second

// coalesceAttempts is how many windows a held evaluation that keeps failing on a transient error is tried over
const coalesceAttempts = 4

// CoalesceConfig holds evaluations that events ask for while one on the same pull request just ran.
type CoalesceConfig struct {
	// Window is how long after an evaluation further ones are held, then run as one. Holding is off when negative.
	Window time.Duration `yaml:"window"`
}

func (c *CoalesceConfig) SetValuesFromEnv(prefix string) {
	setDurationFromEnv(prefix+"WINDOW", &c.Window)
	if c.Window == 0 {
		c.Window = DefaultCoalesceWindow
	}
}

// evaluation is a pull request evaluation that one or more events asked for
type evaluation struct {
	installationID int64
	trigger        common.Trigger
	loc            pull.Locator
	// Workflow runs the events reported, which the runs API may not list yet
	runs []*github.WorkflowRun
	run  func(context.Context, *evaluation) error

	// The context of the first event held, which outlives that event's handling
	ctx      context.Context
	events   int
	attempts int
}

func (e *evaluation) key() string {
	return fmt.Sprintf("%s/%s#%d", e.loc.Owner, e.loc.Repo, e.loc.Number)
}

// absorb adds what other asked for to e
func (e *evaluation) absorb(other *evaluation) {
	e.trigger |= other.trigger
	e.runs = append(e.runs, other.runs...)
	e.events += other.events

	// The later event has the current pull request, unless it only carries part of one we already have in full
	if other.loc.IsComplete() || !e.loc.IsComplete() || other.loc.Value.GetHead().GetSHA() != e.loc.Value.GetHead().GetSHA() {
		e.loc = other.loc
	}
}

// Coalescer runs the first evaluation of a pull request at once and holds any asked for during the window after it.
// Those run as one evaluation when the window closes, which opens another window.
type Coalescer struct {
	window time.Duration
	after  func(time.Duration, func()) *time.Timer

	mu sync.Mutex
	// held is keyed by pull request while its window is open, with a nil value until something is held
	held map[string]*evaluation
}

func NewCoalescer(window time.Duration) *Coalescer {
	return &Coalescer{window: window, after: time.AfterFunc, held: make(map[string]*evaluation)}
}

// Evaluate runs e now, or holds it until the open window on its pull request closes.
// A held evaluation returns nil, as its failures are retried here rather than by the event's retrier.
func (c *Coalescer) Evaluate(ctx context.Context, e *evaluation) error {
	key := e.key()

	c.mu.Lock()
	if _, open := c.held[key]; open {
		c.hold(key, e, ctx)
		c.mu.Unlock()
		zerolog.Ctx(ctx).Info().Msgf("Holding evaluation of pull request '%d' until the window after the last one closes", e.loc.Number)
		return nil
	}
	c.held[key] = nil
	c.mu.Unlock()

	err := e.run(ctx, e)
	c.closeAfterWindow(key)
	return err
}

// hold adds e to what runs when the window on key closes. The caller holds the lock.
func (c *Coalescer) hold(key string, e *evaluation, ctx context.Context) {
	if held := c.held[key]; held != nil {
		held.absorb(e)
		return
	}
	e.ctx = context.WithoutCancel(ctx)
	c.held[key] = e
}

func (c *Coalescer) closeAfterWindow(key string) {
	c.after(c.window, func() { c.closeWindow(key) })
}

// closeWindow runs what was held during the window on key, which opens another, or forgets the key when nothing was
func (c *Coalescer) closeWindow(key string) {
	c.mu.Lock()
	e := c.held[key]
	if e == nil {
		delete(c.held, key)
		c.mu.Unlock()
		return
	}
	c.held[key] = nil
	c.mu.Unlock()

	logger := zerolog.Ctx(e.ctx)
	logger.Info().Msgf("Running the evaluation of pull request '%d' held for %d events", e.loc.Number, e.events)

	err := e.run(e.ctx, e)
	e.attempts++
	switch {
	case err == nil:
	case IsTransient(err) && e.attempts < coalesceAttempts:
		logger.Warn().Err(err).Msgf("Holding evaluation of pull request '%d' for another window after a transient error", e.loc.Number)
		c.mu.Lock()
		c.hold(key, e, e.ctx)
		c.mu.Unlock()
	default:
		logger.Error().Err(err).Msgf("Failed to evaluate pull request '%d'", e.loc.Number)
	}
	c.closeAfterWindow(key)
}
