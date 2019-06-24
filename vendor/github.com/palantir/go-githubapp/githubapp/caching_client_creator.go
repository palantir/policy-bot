// Copyright 2018 Palantir Technologies, Inc.
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
	"fmt"

	"github.com/google/go-github/github"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
)

const (
	DefaultCachingClientCapacity = 64
)

// NewDefaultCachingClientCreator returns a ClientCreator using values from the
// configuration or other defaults.
func NewDefaultCachingClientCreator(c Config, opts ...ClientOption) (ClientCreator, error) {
	delegate := NewClientCreator(
		c.V3APIURL,
		c.V4APIURL,
		c.App.IntegrationID,
		[]byte(c.App.PrivateKey),
		opts...,
	)
	return NewCachingClientCreator(delegate, DefaultCachingClientCapacity)
}

// NewCachingClientCreator returns a ClientCreator that creates a GitHub client for installations of the app specified
// by the provided arguments. It uses an LRU cache of the provided capacity to store clients created for installations
// and returns cached clients when a cache hit exists.
func NewCachingClientCreator(delegate ClientCreator, capacity int) (ClientCreator, error) {
	cache, err := lru.New(capacity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create cache")
	}

	return &cachingClientCreator{
		cachedClients: cache,
		delegate:      delegate,
	}, nil
}

type cachingClientCreator struct {
	cachedClients *lru.Cache
	delegate      ClientCreator
}

func (c *cachingClientCreator) NewAppClient() (*github.Client, error) {
	// app clients are not cached
	return c.delegate.NewAppClient()
}

func (c *cachingClientCreator) NewAppV4Client() (*githubv4.Client, error) {
	// app clients are not cached
	return c.delegate.NewAppV4Client()
}

func (c *cachingClientCreator) NewInstallationClient(installationID int64) (*github.Client, error) {
	// if client is in cache, return it
	key := c.toCacheKey("v3", installationID)
	val, ok := c.cachedClients.Get(key)
	if ok {
		if client, ok := val.(*github.Client); ok {
			return client, nil
		}
	}

	// otherwise, create and return
	client, err := c.delegate.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}
	c.cachedClients.Add(key, client)
	return client, nil
}

func (c *cachingClientCreator) NewInstallationV4Client(installationID int64) (*githubv4.Client, error) {
	// if client is in cache, return it
	key := c.toCacheKey("v4", installationID)
	val, ok := c.cachedClients.Get(key)
	if ok {
		if client, ok := val.(*githubv4.Client); ok {
			return client, nil
		}
	}

	// otherwise, create and return
	client, err := c.delegate.NewInstallationV4Client(installationID)
	if err != nil {
		return nil, err
	}
	c.cachedClients.Add(key, client)
	return client, nil
}

func (c *cachingClientCreator) NewTokenClient(token string) (*github.Client, error) {
	// token clients are not cached
	return c.delegate.NewTokenClient(token)
}

func (c *cachingClientCreator) NewTokenV4Client(token string) (*githubv4.Client, error) {
	// token clients are not cached
	return c.delegate.NewTokenV4Client(token)
}

func (c *cachingClientCreator) toCacheKey(apiVersion string, installationID int64) string {
	return fmt.Sprintf("%s:%d", apiVersion, installationID)
}
