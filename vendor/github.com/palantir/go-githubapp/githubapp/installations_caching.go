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

package githubapp

import (
	"context"
	"fmt"
	"time"

	ttlcache "github.com/patrickmn/go-cache"
)

// NewCachingInstallationsService returns an InstallationsService that always queries GitHub. It should be created with
// a client that authenticates as the target.
// It uses a time based cache of the provided expiry/cleanup time to store app installation info for repositories
// or owners and returns the cached installation info when a cache hit exists.
func NewCachingInstallationsService(delegate InstallationsService, expiry, cleanup time.Duration) InstallationsService {
	return &cachingInstallationsService{
		cache:    ttlcache.New(expiry, cleanup),
		delegate: delegate,
	}
}

type cachingInstallationsService struct {
	cache    *ttlcache.Cache
	delegate InstallationsService
}

func (c *cachingInstallationsService) ListAll(ctx context.Context) ([]Installation, error) {
	// ListAll is not cached due to a lack of keys to retrieve from the cache. Returning all values in the cache is not
	// always desirable
	return c.delegate.ListAll(ctx)
}

func (c *cachingInstallationsService) GetByOwner(ctx context.Context, owner string) (Installation, error) {
	// if installation is in cache, return it
	val, ok := c.cache.Get(owner)
	if ok {
		return val.(Installation), nil
	}

	// otherwise, get installation info, save to cache, and return
	install, err := c.delegate.GetByOwner(ctx, owner)
	if err != nil {
		return Installation{}, err
	}
	c.cache.Set(owner, install, ttlcache.DefaultExpiration)
	return install, nil
}

func (c *cachingInstallationsService) GetByRepository(ctx context.Context, owner, name string) (Installation, error) {
	// if installation is in cache, return it
	key := fmt.Sprintf("%s/%s", owner, name)
	val, ok := c.cache.Get(key)
	if ok {
		return val.(Installation), nil
	}

	// otherwise, get installation info, save to cache, and return
	install, err := c.delegate.GetByRepository(ctx, owner, name)
	if err != nil {
		return Installation{}, err
	}
	c.cache.Set(key, install, ttlcache.DefaultExpiration)
	return install, nil
}
