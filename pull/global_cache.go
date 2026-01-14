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

	"github.com/hairyhenderson/go-codeowners"
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

	// GetCodeowners returns the cached parsed CODEOWNERS for a repository at a
	// specific base branch commit. Since commit SHAs are immutable, caching the
	// parsed content is safe and avoids repeated HTTP requests.
	GetCodeowners(repoID int64, baseRefOID string) (*codeowners.Codeowners, bool)
	SetCodeowners(repoID int64, baseRefOID string, co *codeowners.Codeowners)
}

// LRUGlobalCache is a GlobalCache where each data type is stored in a separate
// LRU cache. This prevents frequently used data of one type from evicting less
// frequently used data of a different type.
type LRUGlobalCache struct {
	pushedAt   *lru.Cache
	codeowners *lru.Cache
}

func NewLRUGlobalCache(pushedAtSize, codeownersSize int) (*LRUGlobalCache, error) {
	pushedAt, err := lru.New(pushedAtSize)
	if err != nil {
		return nil, err
	}
	codeownersCache, err := lru.New(codeownersSize)
	if err != nil {
		return nil, err
	}
	return &LRUGlobalCache{
		pushedAt:   pushedAt,
		codeowners: codeownersCache,
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

func (c *LRUGlobalCache) GetCodeowners(repoID int64, baseRefOID string) (*codeowners.Codeowners, bool) {
	if val, ok := c.codeowners.Get(cacheKey(repoID, baseRefOID)); ok {
		if co, ok := val.(*codeowners.Codeowners); ok {
			return co, true
		}
	}
	return nil, false
}

func (c *LRUGlobalCache) SetCodeowners(repoID int64, baseRefOID string, co *codeowners.Codeowners) {
	c.codeowners.Add(cacheKey(repoID, baseRefOID), co)
}

func cacheKey(repoID int64, identifier string) string {
	return fmt.Sprintf("%d:%s", repoID, identifier)
}
