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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const extendingPolicy = "policy: {approval_extends: {remote: shared/policies, path: parent.yml, ref: main}}"

func TestConfigFetcherParentFailures(t *testing.T) {
	denied := githubErr(http.StatusForbidden)
	for _, test := range []struct {
		name, content, parseError string
		loadError                 error
		undefined                 bool
	}{
		{name: "denied", loadError: denied},
		{name: "error without loader metadata", loadError: denied, undefined: true},
		{name: "undefined", undefined: true},
		{name: "invalid yaml", content: "policy: [", parseError: "failed to parse parent"},
		{name: "unknown field", content: "unexpected: true", parseError: "unexpected"},
		{name: "remote pointer", content: "remote: shared/third", parseError: "must not be a remote reference"},
		{name: "merged remote pointer", content: "<<: {remote: shared/third}", parseError: "must not be a remote reference"},
		{name: "null remote pointer", content: "remote: null", parseError: "must not be a remote reference"},
		{name: "further extension", content: extendingPolicy, parseError: "must not extend another policy"},
		{name: "merged extension", content: "policy: {<<: {approval_extends: {remote: shared/third}}}", parseError: "must not extend another policy"},
		{name: "null extension", content: "policy: {approval_extends: null}", parseError: "approval_extends"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache := NewSeenPolicyCache()
			calls := 0
			fetcher := ConfigFetcher{SeenPolicyCache: cache, Loader: mockConfigLoader{
				loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(extendingPolicy), Source: "shared/policies@main", Path: "child.yml"}, nil
				},
				loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
					calls++
					assert.True(t, cache.Get(SeenPolicyKey{Owner: "original", Repository: "service", BaseBranch: "release"}))
					if test.undefined {
						return appconfig.Config{}, test.loadError
					}
					return appconfig.Config{Content: []byte(test.content), Source: "shared/policies@main", Path: "parent.yml"}, test.loadError
				},
			}}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), nil, "original", "service", "release")
			require.NotNil(t, fc.Config, "merge groups retain the discovered child on parent failures")
			assert.True(t, fc.SeenPolicy)
			assert.Equal(t, "shared/policies@main", fc.Source)
			assert.Equal(t, "child.yml", fc.Path)
			assert.Equal(t, 1, calls, "never load a grandparent or fall back to defaults")
			switch {
			case test.loadError != nil:
				require.ErrorIs(t, fc.LoadError, test.loadError)
				if test.undefined {
					assert.Contains(t, fc.LoadError.Error(), "child.yml")
				} else {
					assert.Contains(t, fc.LoadError.Error(), "parent.yml")
				}
				assert.Contains(t, fc.LoadError.Error(), "shared/policies@main")
			case test.undefined:
				require.ErrorContains(t, fc.LoadError, "does not exist")
			default:
				require.ErrorContains(t, fc.ParseError, test.parseError)
				assert.Contains(t, fc.ParseError.Error(), "parent.yml")
				assert.Contains(t, fc.ParseError.Error(), "shared/policies@main")
			}
			ec := makeEvalContext(false)
			ec.Config = fc
			evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			require.Error(t, err)
			assert.Nil(t, evaluator)
			require.NotNil(t, ec.Status, "the first evaluation with a broken parent must report a policy error")
			assert.Equal(t, "error", ec.Status.GetState())
		})
	}
}

func TestConfigFetcherInvalidExtensionDoesNotLoadParent(t *testing.T) {
	for _, input := range []string{
		"policy: {approval: [], approval_extends: {remote: shared/policies}}",
		"policy: {approval: null, approval_extends: {remote: shared/policies}}",
		"policy: {approval_extends: null}",
		"policy: {approval_extends: {}}",
		"policy: {approval_extends: {remote: 'shared/policies/contents/other#'}}",
	} {
		t.Run(input, func(t *testing.T) {
			fetcher := ConfigFetcher{Loader: mockConfigLoader{
				loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(input), Source: "original/service@release", Path: ".policy.yml"}, nil
				},
				loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
					t.Error("invalid extension requested a parent")
					return appconfig.Config{}, nil
				},
			}}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), nil, "original", "service", "release")
			require.Error(t, fc.ParseError)
			assert.Nil(t, fc.Config)
			assert.True(t, fc.SeenPolicy)
			assert.Equal(t, "original/service@release", fc.Source)
		})
	}
}

func TestConfigFetcherParentRetry(t *testing.T) {
	for _, test := range []struct {
		name        string
		successCall int
		cancel      bool
		timeout     bool
		wantCalls   int
	}{
		{name: "transient error", successCall: 2, wantCalls: 2},
		{name: "wrapped timeout", timeout: true, successCall: 2, wantCalls: 2},
		{name: "bounded retries", wantCalls: 4},
		{name: "cancel backoff", cancel: true, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls, childCalls := 0, 0
				failure := fmt.Errorf("read file: %w", githubErr(http.StatusBadGateway))
				if test.timeout {
					failure = fmt.Errorf("read file: %w", context.DeadlineExceeded)
				}
				fetcher := ConfigFetcher{Loader: mockConfigLoader{
					loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
						childCalls++
						return appconfig.Config{Content: []byte(extendingPolicy), Source: "original/service@release", Path: ".policy.yml"}, nil
					},
					loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
						calls++
						c := appconfig.Config{Content: []byte("policy: {approval: []}"), Source: "shared/policies@main", Path: "parent.yml"}
						if calls == test.successCall {
							return c, nil
						}
						if test.cancel {
							go func() {
								time.Sleep(10 * time.Millisecond)
								cancel()
							}()
						}
						return c, failure
					},
				}}
				fc := fetcher.ConfigForRepositoryBranch(ctx, nil, "original", "service", "release")
				assert.Equal(t, test.wantCalls, calls)
				assert.Equal(t, 1, childCalls)
				require.NotNil(t, fc.Config)
				assert.True(t, fc.SeenPolicy)
				if test.successCall != 0 {
					require.NoError(t, fc.LoadError)
					require.NotNil(t, fc.Parent)
				} else if test.cancel {
					require.ErrorIs(t, fc.LoadError, context.Canceled)
				} else {
					require.ErrorIs(t, fc.LoadError, failure)
					assert.True(t, isServerError(fc.LoadError))
				}
			})
		})
	}
}

func TestConfigFetcherExtensionDiscovery(t *testing.T) {
	for _, test := range []struct {
		name, local, shared string
		forceShared         bool
		requests            []string
	}{
		{name: "local", local: extendingPolicy, requests: []string{"/repos/original/service/contents/.policy.yml?ref=release", "/repos/shared/policies/contents/parent.yml?ref=main"}},
		{name: "remote child", local: "remote: shared/policies\npath: child.yml\nref: main", requests: []string{"/repos/original/service/contents/.policy.yml?ref=release", "/repos/shared/policies/contents/child.yml?ref=main", "/repos/shared/policies/contents/parent.yml?ref=main"}},
		{name: "owner default", shared: extendingPolicy, requests: []string{"/repos/original/service/contents/.policy.yml?ref=release", "/repos/original/.github", "/repos/original/.github/contents/default.yml?ref=main", "/repos/shared/policies/contents/parent.yml?ref=main"}},
		{name: "forced shared", local: extendingPolicy, shared: "policy: {approval: []}", forceShared: true, requests: []string{"/repos/original/.github", "/repos/original/.github/contents/default.yml?ref=main"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests []string
			client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.RequestURI())
				switch r.URL.Path {
				case "/repos/original/service/contents/.policy.yml":
					if test.local == "" {
						http.NotFound(w, r)
						return
					}
					writePolicyFile(t, w, test.local)
				case "/repos/original/.github":
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"name": ".github", "default_branch": "main"}))
				case "/repos/original/.github/contents/default.yml":
					writePolicyFile(t, w, test.shared)
				case "/repos/shared/policies/contents/child.yml":
					writePolicyFile(t, w, extendingPolicy)
				case "/repos/shared/policies/contents/parent.yml":
					writePolicyFile(t, w, "policy: {approval: []}")
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			})
			paths := []string{".policy.yml"}
			if test.forceShared {
				paths = nil
			}
			fetcher := ConfigFetcher{Loader: appconfig.NewLoader(paths, appconfig.WithOwnerDefault(".github", []string{"default.yml"}))}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), client, "original", "service", "release")
			require.NoError(t, fc.LoadError)
			require.NoError(t, fc.ParseError)
			assert.Equal(t, test.requests, requests)
			assert.Equal(t, !test.forceShared, fc.Parent != nil)
		})
	}
}

func TestConfigFetcherOriginalOwnerForPrivateParent(t *testing.T) {
	for _, test := range []struct {
		name            string
		private, denied bool
		lookupError     error
	}{
		{name: "private child and parent", private: true},
		{name: "public without switching"},
		{name: "public fallback", private: true, lookupError: errors.New("installation unavailable")},
		{name: "parent denied without fallback", private: true, denied: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalCalls, selectedCalls := 0, 0
			serve := func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/shared/policies/contents/child.yml":
					writePolicyFile(t, w, extendingPolicy)
				case "/repos/shared/policies/contents/parent.yml":
					if test.denied {
						http.Error(w, "denied", http.StatusForbidden)
						return
					}
					writePolicyFile(t, w, "policy: {approval: []}")
				default:
					t.Errorf("unexpected remote request: %s", r.URL)
					http.NotFound(w, r)
				}
			}
			original := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				originalCalls++
				if r.URL.Path == "/repos/original/service/contents/.policy.yml" {
					writePolicyFile(t, w, "remote: shared/policies\npath: child.yml\nref: main")
					return
				}
				if test.private && test.lookupError == nil {
					t.Error("private remote used the original installation client")
				}
				serve(w, r)
			})
			selected := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				selectedCalls++
				serve(w, r)
			})
			installations := &extensionInstallations{err: test.lookupError}
			var options []appconfig.Option
			if test.private {
				options = append(options, appconfig.WithPrivateRemotes(testClients{client: selected}, installations))
			}
			fetcher := ConfigFetcher{Loader: appconfig.NewLoader([]string{".policy.yml"}, options...)}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), original, "original", "service", "release")
			require.NoError(t, fc.ParseError)
			if test.denied {
				var responseError *github.ErrorResponse
				require.ErrorAs(t, fc.LoadError, &responseError)
				assert.Equal(t, http.StatusForbidden, responseError.Response.StatusCode)
			} else {
				require.NoError(t, fc.LoadError)
				require.NotNil(t, fc.Parent)
			}
			if test.private {
				assert.Equal(t, []string{"shared/policies", "shared/policies"}, installations.lookups)
			} else {
				assert.Empty(t, installations.lookups)
			}
			if test.private && test.lookupError == nil {
				assert.Equal(t, 1, originalCalls)
				assert.Equal(t, 2, selectedCalls)
			} else {
				assert.Equal(t, 3, originalCalls)
				assert.Zero(t, selectedCalls)
			}
		})
	}
}

func TestConfigFetcherMissingOrEmptyParent(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		directory bool
	}{
		{name: "missing", status: http.StatusNotFound},
		{name: "directory", status: http.StatusOK, directory: true},
		{name: "empty literal file", status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/repos/original/service/contents/.policy.yml" {
					writePolicyFile(t, w, extendingPolicy)
					return
				}
				assert.Equal(t, "/repos/shared/policies/contents/parent.yml?ref=main", r.URL.RequestURI())
				w.WriteHeader(test.status)
				if test.directory {
					_, err := w.Write([]byte("[]"))
					assert.NoError(t, err)
				} else if test.status == http.StatusOK {
					writePolicyFile(t, w, "")
				}
			})
			fetcher := ConfigFetcher{Loader: appconfig.NewLoader([]string{".policy.yml", "fallback.yml"})}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), client, "original", "service", "release")
			assert.Equal(t, 2, calls)
			require.NotNil(t, fc.Config)
			if test.directory || test.status != http.StatusOK {
				require.ErrorContains(t, fc.LoadError, "file does not exist")
				assert.Nil(t, fc.Parent)
			} else {
				require.NoError(t, fc.LoadError)
				require.NoError(t, fc.ParseError)
				require.NotNil(t, fc.Parent)
			}
		})
	}
}

func TestConfigFetcherParentNetworkCancellation(t *testing.T) {
	requested := make(chan struct{})
	client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/original/service/contents/.policy.yml" {
			writePolicyFile(t, w, extendingPolicy)
			return
		}
		assert.Equal(t, "/repos/shared/policies/contents/parent.yml?ref=main", r.URL.RequestURI())
		close(requested)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fetcher := ConfigFetcher{Loader: appconfig.NewLoader([]string{".policy.yml"})}
	result := make(chan FetchedConfig, 1)
	go func() {
		result <- fetcher.ConfigForRepositoryBranch(ctx, client, "original", "service", "release")
	}()
	select {
	case <-requested:
	case <-ctx.Done():
		t.Fatal("parent request did not begin")
	}
	cancel()
	select {
	case fc := <-result:
		require.ErrorIs(t, fc.LoadError, context.Canceled)
		require.NotNil(t, fc.Config)
		assert.True(t, fc.SeenPolicy)
		assert.Contains(t, fc.LoadError.Error(), "parent.yml")
	case <-time.After(5 * time.Second):
		t.Fatal("parent request did not stop on cancellation")
	}
}

func extensionTestClient(t *testing.T, handler http.HandlerFunc) *github.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&baseURL, nil))
	require.NoError(t, err)
	return client
}

func writePolicyFile(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{
		"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content)),
	}))
}

type extensionInstallations struct {
	githubapp.InstallationsService
	lookups []string
	err     error
}

func (i *extensionInstallations) GetByRepository(_ context.Context, owner, repository string) (githubapp.Installation, error) {
	i.lookups = append(i.lookups, owner+"/"+repository)
	return githubapp.Installation{ID: 2}, i.err
}
