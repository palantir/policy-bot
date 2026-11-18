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
	"sync"
	"time"
)

// cachedConfigEntry stores a FetchedConfig that was successfully loaded at some
// point, along with the time at which the entry expires.
type cachedConfigEntry struct {
	config  FetchedConfig
	expires time.Time
}

// LastGoodConfigCache stores the last successfully loaded FetchedConfig for
// each repo branch. When a transient error (GitHub 5xx or timeout) prevents
// policy-bot from loading a policy, the cache can serve the last known good
// config instead of failing the evaluation entirely.
//
// A TTL limits how long a stale config is served so that policy changes
// eventually take effect. A TTL of zero or less disables the cache.
type LastGoodConfigCache struct {
	mu      sync.RWMutex
	entries map[SeenPolicyKey]cachedConfigEntry
	ttl     time.Duration
}

// NewLastGoodConfigCache creates a LastGoodConfigCache with the given TTL.
// A ttl of zero or less disables all caching (Get always returns false, Set
// is a no-op).
func NewLastGoodConfigCache(ttl time.Duration) *LastGoodConfigCache {
	return &LastGoodConfigCache{
		entries: make(map[SeenPolicyKey]cachedConfigEntry),
		ttl:     ttl,
	}
}

// Get returns the cached FetchedConfig for the given key if one exists and
// has not expired. Returns false if no entry exists, the entry has expired,
// or the cache is disabled.
func (c *LastGoodConfigCache) Get(key SeenPolicyKey) (FetchedConfig, bool) {
	if c == nil || c.ttl <= 0 {
		return FetchedConfig{}, false
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expires) {
		return FetchedConfig{}, false
	}
	return entry.config, true
}

// Set stores a FetchedConfig for the given key with an expiration of now+TTL.
// Is a no-op if the cache is disabled.
func (c *LastGoodConfigCache) Set(key SeenPolicyKey, fc FetchedConfig) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	c.entries[key] = cachedConfigEntry{
		config:  fc,
		expires: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}
