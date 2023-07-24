// Copyright 2023 Palantir Technologies, Inc.
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

package pull

import (
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

// GlobalCache implementations provide a way to cache values that are safe to
// cache at the application level. Values in the global cache should not become
// stale due to external changes and should only expire to prevent the cache
// from becoming infinitely large.
type GlobalCache interface {
	GetPushedAt(repoID int64, sha string) (time.Time, bool)
	SetPushedAt(repoID int64, sha string, t time.Time)
}

// LRUGlobalCache is a GlobalCache where each data type is stored in a separate
// LRU cache. This prevents frequently used data of one type from evicting less
// frequently used data of a different type.
type LRUGlobalCache struct {
	pushedAt *lru.Cache
}

func NewLRUGlobalCache(pushedAtSize int) (*LRUGlobalCache, error) {
	pushedAt, err := lru.New(pushedAtSize)
	if err != nil {
		return nil, err
	}
	return &LRUGlobalCache{pushedAt: pushedAt}, nil
}

func (c *LRUGlobalCache) GetPushedAt(repoID int64, sha string) (time.Time, bool) {
	if val, ok := c.pushedAt.Get(pushedAtKey(repoID, sha)); ok {
		if t, ok := val.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c *LRUGlobalCache) SetPushedAt(repoID int64, sha string, t time.Time) {
	c.pushedAt.Add(pushedAtKey(repoID, sha), t)
}

func pushedAtKey(repoID int64, sha string) string {
	return fmt.Sprintf("%d:%s", repoID, sha)
}
