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
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitDeferrerCoalescesByInstallationAndKey(t *testing.T) {
	deferrer := NewRateLimitDeferrer(0)
	deferrer.jitter = func() time.Duration { return 0 }

	var (
		mu       sync.Mutex
		triggers []common.Trigger
	)
	run := func(_ context.Context, trigger common.Trigger) {
		mu.Lock()
		defer mu.Unlock()
		triggers = append(triggers, trigger)
	}

	resetAt := time.Now().Add(25 * time.Millisecond)
	deferrer.DeferUntil(context.Background(), 123, "repo/pr/1", common.TriggerCommit, resetAt, run)
	assert.True(t, deferrer.DeferIfActive(context.Background(), 123, "repo/pr/1", common.TriggerStatus, run))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(triggers) == 1
	}, time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, common.TriggerCommit|common.TriggerStatus, triggers[0])
}

func TestRateLimitDeferrerOnlyDefersMatchingInstallation(t *testing.T) {
	deferrer := NewRateLimitDeferrer(0)
	deferrer.jitter = func() time.Duration { return 0 }

	deferrer.DeferUntil(context.Background(), 123, "repo/pr/1", common.TriggerCommit, time.Now().Add(time.Minute), func(context.Context, common.Trigger) {})

	assert.True(t, deferrer.DeferIfActive(context.Background(), 123, "repo/pr/2", common.TriggerStatus, func(context.Context, common.Trigger) {}))
	assert.False(t, deferrer.DeferIfActive(context.Background(), 456, "repo/pr/1", common.TriggerStatus, func(context.Context, common.Trigger) {}))
}

func TestRateLimitResetTimeRecognizesPrimaryAndSecondaryLimits(t *testing.T) {
	resetAt := time.Date(2026, 6, 4, 8, 34, 33, 0, time.UTC)
	primary := newRateLimitError(resetAt)

	gotReset, ok := rateLimitResetTime(primary)
	require.True(t, ok)
	assert.Equal(t, resetAt, gotReset)

	retryAfter := 7 * time.Second
	secondary := &github.AbuseRateLimitError{
		Response:   rateLimitResponse(http.StatusForbidden),
		Message:    "secondary rate limit",
		RetryAfter: &retryAfter,
	}
	before := time.Now()
	gotReset, ok = rateLimitResetTime(secondary)
	require.True(t, ok)
	assert.WithinDuration(t, before.Add(retryAfter), gotReset, time.Second)
}

func newRateLimitError(resetAt time.Time) *github.RateLimitError {
	return &github.RateLimitError{
		Rate: github.Rate{
			Limit:     9600,
			Remaining: 0,
			Reset:     github.Timestamp{Time: resetAt},
		},
		Response: rateLimitResponse(http.StatusForbidden),
		Message:  "API rate limit exceeded",
	}
}

func rateLimitResponse(status int) *http.Response {
	reqURL, _ := url.Parse("https://api.github.com/repos/kaiko-ai/kaiko-eng/contents/.policy.yml?ref=main")
	return &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    reqURL,
		},
		Header: make(http.Header),
	}
}
