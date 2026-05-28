// Copyright 2025 Palantir Technologies, Inc.
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

package githubclient_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/server/githubclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestKey produces a minimal PEM-encoded RSA private key for tests.
func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// fakeGitHub serves the minimal GitHub App token endpoint.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "test-token",
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		})
	})
	return httptest.NewServer(mux)
}

func testConfig(t *testing.T, serverURL string) githubapp.Config {
	t.Helper()
	key := generateTestKey(t)
	return githubapp.Config{
		V3APIURL: serverURL + "/",
		V4APIURL: serverURL + "/graphql",
		App: struct {
			IntegrationID int64  `yaml:"integration_id" json:"integrationId"`
			WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
			PrivateKey    string `yaml:"private_key" json:"privateKey"`
		}{
			IntegrationID: 1,
			PrivateKey:    string(key),
		},
	}
}

func TestNewInvalidatingClientCreator_MissingURLs(t *testing.T) {
	key := generateTestKey(t)
	cfg := githubapp.Config{
		App: struct {
			IntegrationID int64  `yaml:"integration_id" json:"integrationId"`
			WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
			PrivateKey    string `yaml:"private_key" json:"privateKey"`
		}{
			IntegrationID: 1,
			PrivateKey:    string(key),
		},
	}
	_, err := githubclient.NewInvalidatingClientCreator(cfg, nil)
	assert.Error(t, err)
}

func TestInstallationIDContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	id, ok := githubclient.InstallationIDFromContext(ctx)
	assert.False(t, ok)
	assert.Zero(t, id)

	ctx2 := githubclient.ContextWithInstallationID(ctx, 42)
	id, ok = githubclient.InstallationIDFromContext(ctx2)
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)

	// original context is unchanged
	_, ok = githubclient.InstallationIDFromContext(ctx)
	assert.False(t, ok)
}

func TestSetInstallationIDMiddleware_SetsOnRequest(t *testing.T) {
	var captured int64
	var capturedOK bool

	inner := testRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		captured, capturedOK = githubclient.InstallationIDFromContext(r.Context())
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	mw := githubclient.SetInstallationIDMiddleware(99)
	rt := mw(inner)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	assert.True(t, capturedOK)
	assert.Equal(t, int64(99), captured)
}

func TestNewInstallationClient_CachesResult(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	c1, err := creator.NewInstallationClient(1)
	require.NoError(t, err)

	c2, err := creator.NewInstallationClient(1)
	require.NoError(t, err)

	assert.Same(t, c1, c2, "expected cached client to be returned on second call")
}

func TestNewInstallationClient_DifferentIDsIndependent(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	c1, err := creator.NewInstallationClient(1)
	require.NoError(t, err)

	c2, err := creator.NewInstallationClient(2)
	require.NoError(t, err)

	assert.NotSame(t, c1, c2)
}

func TestInvalidate_ForcesNewClient(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	c1, err := creator.NewInstallationClient(1)
	require.NoError(t, err)

	creator.Invalidate(1)

	c2, err := creator.NewInstallationClient(1)
	require.NoError(t, err)

	assert.NotSame(t, c1, c2, "expected a new client after invalidation")
}

func TestInvalidate_SafeWhenNoEntry(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	// must not panic
	assert.NotPanics(t, func() { creator.Invalidate(999) })
}

func TestInvalidate_EvictsBothV3AndV4(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	v3Before, err := creator.NewInstallationClient(1)
	require.NoError(t, err)
	v4Before, err := creator.NewInstallationV4Client(1)
	require.NoError(t, err)

	creator.Invalidate(1)

	v3After, err := creator.NewInstallationClient(1)
	require.NoError(t, err)
	v4After, err := creator.NewInstallationV4Client(1)
	require.NoError(t, err)

	assert.NotSame(t, v3Before, v3After)
	assert.NotSame(t, v4Before, v4After)
}

func TestConcurrentNewInstallationClient_ReturnsSameClient(t *testing.T) {
	srv := fakeGitHub(t)
	defer srv.Close()

	creator, err := githubclient.NewInvalidatingClientCreator(testConfig(t, srv.URL), nil)
	require.NoError(t, err)

	// Establish the canonical cached client before concurrent goroutines run.
	want, err := creator.NewInstallationClient(7)
	require.NoError(t, err)

	const goroutines = 20
	clients := make([]*github.Client, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			c, err2 := creator.NewInstallationClient(7)
			if err2 == nil {
				clients[idx] = c
			}
		}()
	}
	wg.Wait()

	for _, c := range clients {
		assert.Same(t, want, c, "concurrent callers must receive the same cached *github.Client")
	}
}

// testRoundTripperFunc is a local test helper implementing http.RoundTripper.
type testRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f testRoundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
