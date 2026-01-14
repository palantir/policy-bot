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
	// GetPushedAt returns the cached push timestamp for a commit.
	GetPushedAt(repoID int64, sha string) (time.Time, bool)
	SetPushedAt(repoID int64, sha string, t time.Time)

	// GetCodeownersPath returns the cached CODEOWNERS file path for a repository
	// at a specific base branch commit. Caching the path avoids repeated 404
	// requests when checking standard CODEOWNERS locations.
	GetCodeownersPath(repoID int64, baseRefOID string) (string, bool)
	SetCodeownersPath(repoID int64, baseRefOID string, path string)
}

// LRUGlobalCache is a GlobalCache where each data type is stored in a separate
// LRU cache. This prevents frequently used data of one type from evicting less
// frequently used data of a different type.
type LRUGlobalCache struct {
	pushedAt       *lru.Cache
	codeownersPath *lru.Cache
}

func NewLRUGlobalCache(pushedAtSize, codeownersPathSize int) (*LRUGlobalCache, error) {
	pushedAt, err := lru.New(pushedAtSize)
	if err != nil {
		return nil, err
	}
	codeownersPath, err := lru.New(codeownersPathSize)
	if err != nil {
		return nil, err
	}
	return &LRUGlobalCache{
		pushedAt:       pushedAt,
		codeownersPath: codeownersPath,
	}, nil
}

func (c *LRUGlobalCache) GetPushedAt(repoID int64, sha string) (time.Time, bool) {
	if val, ok := c.pushedAt.Get(cacheKey(repoID, sha)); ok {
		if t, ok := val.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c *LRUGlobalCache) SetPushedAt(repoID int64, sha string, t time.Time) {
	c.pushedAt.Add(cacheKey(repoID, sha), t)
}

func (c *LRUGlobalCache) GetCodeownersPath(repoID int64, baseRefOID string) (string, bool) {
	if val, ok := c.codeownersPath.Get(cacheKey(repoID, baseRefOID)); ok {
		if path, ok := val.(string); ok {
			return path, true
		}
	}
	return "", false
}

func (c *LRUGlobalCache) SetCodeownersPath(repoID int64, baseRefOID string, path string) {
	c.codeownersPath.Add(cacheKey(repoID, baseRefOID), path)
}

func cacheKey(repoID int64, identifier string) string {
	return fmt.Sprintf("%d:%s", repoID, identifier)
}
