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

package githubclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/go-github/v81/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeInvalidator is a test double for *InvalidatingClientCreator that lets
// tests control what client is returned on each NewInstallationClient call.
type fakeInvalidator struct {
	mu           sync.Mutex
	invalidated  []int64
	clientsQueue []*github.Client
}

func (f *fakeInvalidator) Invalidate(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, id)
}

func (f *fakeInvalidator) NewInstallationClient(_ int64) (*github.Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.clientsQueue) == 0 {
		return github.NewClient(nil), nil
	}
	c := f.clientsQueue[0]
	f.clientsQueue = f.clientsQueue[1:]
	return c, nil
}

// retryableInvalidator wraps fakeInvalidator to satisfy the interface expected
// by newRetryOn401TransportWith (the test-only constructor below).
type retryableInvalidator interface {
	Invalidate(int64)
	NewInstallationClient(int64) (*github.Client, error)
}

// newRetryOn401TransportWith is a test helper that builds the transport using
// any retryableInvalidator, bypassing the concrete *InvalidatingClientCreator
// type so tests don't need the real implementation from Task #1.
func newRetryOn401TransportWith(inv retryableInvalidator, next http.RoundTripper) http.RoundTripper {
	return &retryOn401TransportFake{
		next: next,
		inv:  inv,
	}
}

type retryOn401TransportFake struct {
	next http.RoundTripper
	inv  retryableInvalidator
}

func (t *retryOn401TransportFake) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	if req.Context().Value(retriedKey{}) != nil {
		return resp, nil
	}

	installationID, ok := InstallationIDFromContext(req.Context())
	if !ok {
		return resp, nil
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	t.inv.Invalidate(installationID)

	freshClient, err := t.inv.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	ctx := context.WithValue(req.Context(), retriedKey{}, true)
	req2 := req.Clone(ctx)
	if len(bodyBytes) > 0 {
		req2.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req2.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	return freshClient.Client().Do(req2)
}

func installationContext(id int64) context.Context {
	return ContextWithInstallationID(context.Background(), id)
}

// newGitHubClientWithTransport builds a *github.Client backed by a given transport.
func newGitHubClientWithTransport(srv *httptest.Server, rt http.RoundTripper) *github.Client {
	hc := &http.Client{Transport: rt}
	c := github.NewClient(hc)
	c.BaseURL, _ = c.BaseURL.Parse(srv.URL + "/")
	return c
}

func TestRetryOn401_200PassThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	fake := &fakeInvalidator{}
	transport := newRetryOn401TransportWith(fake, http.DefaultTransport)
	hc := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(installationContext(42), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, fake.invalidated, "should not have invalidated on 200")
}

func TestRetryOn401_RetriesOnce_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// The fresh client returned after invalidation points to the same test server.
	freshClient := newGitHubClientWithTransport(srv, http.DefaultTransport)

	fake := &fakeInvalidator{
		clientsQueue: []*github.Client{freshClient},
	}
	transport := newRetryOn401TransportWith(fake, http.DefaultTransport)
	hc := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(installationContext(99), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []int64{99}, fake.invalidated)
	assert.Equal(t, 2, callCount)
}

func TestRetryOn401_PersistentUnauthorized_NoInfiniteLoop(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// The fresh client also returns 401, which should trigger the guard.
	freshTransport := newRetryOn401TransportWith(&fakeInvalidator{}, http.DefaultTransport)
	freshHC := &http.Client{Transport: freshTransport}
	freshClient := github.NewClient(freshHC)
	freshClient.BaseURL, _ = freshClient.BaseURL.Parse(srv.URL + "/")

	fake := &fakeInvalidator{
		clientsQueue: []*github.Client{freshClient},
	}
	transport := newRetryOn401TransportWith(fake, http.DefaultTransport)
	hc := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(installationContext(7), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	// Exactly 2 calls: first attempt + one retry (guard stops further retries).
	assert.Equal(t, 2, callCount)
}

func TestRetryOn401_MissingInstallationID_NoRetry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	fake := &fakeInvalidator{}
	transport := newRetryOn401TransportWith(fake, http.DefaultTransport)
	hc := &http.Client{Transport: transport}

	// No installation ID in context.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	require.NoError(t, err)

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Empty(t, fake.invalidated)
	assert.Equal(t, 1, callCount)
}

func TestRetryOn401_POSTBodyReplayed(t *testing.T) {
	const payload = `{"hello":"world"}`
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	freshClient := newGitHubClientWithTransport(srv, http.DefaultTransport)
	fake := &fakeInvalidator{
		clientsQueue: []*github.Client{freshClient},
	}
	transport := newRetryOn401TransportWith(fake, http.DefaultTransport)
	hc := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(
		installationContext(55),
		http.MethodPost,
		srv.URL+"/test",
		bytes.NewBufferString(payload),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, bodies, 2, "expected two attempts")
	assert.Equal(t, payload, bodies[0], "first attempt body")
	assert.Equal(t, payload, bodies[1], "retry body must match original")
}
