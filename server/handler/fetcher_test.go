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
	"encoding/base64"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFetcherCachesSuccessfulConfig(t *testing.T) {
	transport := &configCountingTransport{}
	client := githubClientForTransport(t, transport)
	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	fetcher := NewConfigFetcher(appconfig.NewLoader([]string{".policy.yml"}))
	fetcher.Clock = func() time.Time { return now }
	fetcher.CacheTTL = time.Minute

	first := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")
	second := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")

	require.NoError(t, first.LoadError)
	require.NoError(t, second.LoadError)
	require.NotNil(t, first.Config)
	require.NotNil(t, second.Config)
	assert.NotSame(t, first.Config, second.Config, "cached configs should be cloned before reuse")
	assert.Equal(t, 1, transport.requestCount())

	now = now.Add(time.Minute + time.Second)
	third := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")

	require.NoError(t, third.LoadError)
	assert.Equal(t, 2, transport.requestCount())
}

func TestConfigFetcherSingleflightsConcurrentLoads(t *testing.T) {
	transport := &configCountingTransport{delay: 50 * time.Millisecond}
	client := githubClientForTransport(t, transport)
	fetcher := NewConfigFetcher(appconfig.NewLoader([]string{".policy.yml"}))
	fetcher.CacheTTL = time.Minute

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")
			errs <- fc.LoadError
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, transport.requestCount())
}

func TestConfigFetcherDoesNotCacheLoadErrors(t *testing.T) {
	transport := &configCountingTransport{status: http.StatusForbidden}
	client := githubClientForTransport(t, transport)
	fetcher := NewConfigFetcher(appconfig.NewLoader([]string{".policy.yml"}))
	fetcher.CacheTTL = time.Minute

	first := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")
	second := fetcher.ConfigForRepositoryBranch(context.Background(), client, "kaiko-ai", "kaiko-eng", "main")

	require.Error(t, first.LoadError)
	require.Error(t, second.LoadError)
	assert.Equal(t, 2, transport.requestCount())
}

type configCountingTransport struct {
	mu       sync.Mutex
	requests int
	delay    time.Duration
	status   int
}

func (t *configCountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}

	t.mu.Lock()
	t.requests++
	t.mu.Unlock()

	status := t.status
	if status == 0 {
		status = http.StatusOK
	}
	if status != http.StatusOK {
		return jsonResponse(req, status, `{"message":"boom"}`), nil
	}

	content := base64.StdEncoding.EncodeToString([]byte("{}\n"))
	return jsonResponse(req, http.StatusOK, `{"type":"file","encoding":"base64","content":"`+content+`","name":".policy.yml","path":".policy.yml"}`), nil
}

func (t *configCountingTransport) requestCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requests
}

func githubClientForTransport(t *testing.T, transport http.RoundTripper) *github.Client {
	t.Helper()

	client := github.NewClient(&http.Client{Transport: transport})
	baseURL, err := url.Parse("http://github.localhost/")
	require.NoError(t, err)
	client.BaseURL = baseURL
	return client
}
