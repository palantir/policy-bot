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

	"github.com/google/go-github/v90/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// fakeInstallations is a minimal githubapp.InstallationsService for tests.
type fakeInstallations struct {
	byOwner map[string]githubapp.Installation
}

func (f fakeInstallations) ListAll(ctx context.Context) ([]githubapp.Installation, error) {
	return nil, errors.New("fakeInstallations: ListAll not implemented")
}

func (f fakeInstallations) GetByOwner(ctx context.Context, owner string) (githubapp.Installation, error) {
	inst, ok := f.byOwner[owner]
	if !ok {
		return githubapp.Installation{}, githubapp.InstallationNotFound(owner)
	}
	return inst, nil
}

func (f fakeInstallations) GetByRepository(ctx context.Context, owner, repo string) (githubapp.Installation, error) {
	return githubapp.Installation{}, errors.New("fakeInstallations: GetByRepository not implemented")
}

// fakeClientCreator is a minimal githubapp.ClientCreator for tests. Only
// NewInstallationClient is expected to be called by CrossOrgConfigLoader;
// every other method returns an error if invoked.
type fakeClientCreator struct {
	byInstallationID map[int64]*github.Client
}

func (f fakeClientCreator) NewAppClient() (*github.Client, error) {
	return nil, errors.New("fakeClientCreator: NewAppClient not implemented")
}

func (f fakeClientCreator) NewAppV4Client() (*githubv4.Client, error) {
	return nil, errors.New("fakeClientCreator: NewAppV4Client not implemented")
}

func (f fakeClientCreator) NewInstallationClient(installationID int64) (*github.Client, error) {
	client, ok := f.byInstallationID[installationID]
	if !ok {
		return nil, fmt.Errorf("fakeClientCreator: no client configured for installation %d", installationID)
	}
	return client, nil
}

func (f fakeClientCreator) NewInstallationV4Client(installationID int64) (*githubv4.Client, error) {
	return nil, errors.New("fakeClientCreator: NewInstallationV4Client not implemented")
}

func (f fakeClientCreator) NewTokenSourceClient(ts oauth2.TokenSource) (*github.Client, error) {
	return nil, errors.New("fakeClientCreator: NewTokenSourceClient not implemented")
}

func (f fakeClientCreator) NewTokenSourceV4Client(ts oauth2.TokenSource) (*githubv4.Client, error) {
	return nil, errors.New("fakeClientCreator: NewTokenSourceV4Client not implemented")
}

func (f fakeClientCreator) NewTokenClient(token string) (*github.Client, error) {
	return nil, errors.New("fakeClientCreator: NewTokenClient not implemented")
}

func (f fakeClientCreator) NewTokenV4Client(token string) (*githubv4.Client, error) {
	return nil, errors.New("fakeClientCreator: NewTokenV4Client not implemented")
}

// newTestGitHubClient returns a *github.Client that sends requests to a
// local httptest server driven by mux, and registers server.Close via
// t.Cleanup.
func newTestGitHubClient(t *testing.T, mux *http.ServeMux) *github.Client {
	t.Helper()

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	require.NoError(t, err)
	return client
}

func TestCrossOrgConfigLoader_LocalPolicyPassthrough(t *testing.T) {
	localCfg := appconfig.Config{
		Content: []byte("policy:\n  approval: []\n"),
		Source:  "testorg/testrepo@main",
		Path:    ".policy.yml",
	}

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				return localCfg, nil
			},
		},
		paths:         []string{".policy.yml"},
		clientCreator: fakeClientCreator{},
		installations: fakeInstallations{},
		newRemoteLoader: func(paths []string) ConfigLoader {
			t.Fatal("newRemoteLoader should not be called for a non-remote local policy")
			return nil
		},
	}

	got, err := loader.LoadConfig(context.Background(), nil, "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, localCfg, got)
}

func TestCrossOrgConfigLoader_SameOrgRemoteReusesClient(t *testing.T) {
	requestingClient := newTestGitHubClient(t, http.NewServeMux())

	var capturedClient *github.Client
	var capturedOwner, capturedRepo, capturedRef string

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				return appconfig.Config{
					Content: []byte("remote: testorg/shared-policy\nref: main\n"),
					Source:  "testorg/testrepo@main",
					Path:    ".policy.yml",
				}, nil
			},
		},
		paths:         []string{".policy.yml"},
		clientCreator: fakeClientCreator{},
		installations: fakeInstallations{},
		newRemoteLoader: func(paths []string) ConfigLoader {
			return mockConfigLoader{
				loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
					capturedClient = client
					capturedOwner, capturedRepo, capturedRef = owner, repo, ref
					return appconfig.Config{
						Content: []byte("policy:\n  approval: []\n"),
					}, nil
				},
			}
		},
	}

	got, err := loader.LoadConfig(context.Background(), requestingClient, "testorg", "testrepo", "main")
	require.NoError(t, err)

	assert.Same(t, requestingClient, capturedClient, "same-org remote must reuse the requesting client, not look up an installation")
	assert.Equal(t, "testorg", capturedOwner)
	assert.Equal(t, "shared-policy", capturedRepo)
	assert.Equal(t, "main", capturedRef)

	assert.Equal(t, "testorg/shared-policy@main", got.Source)
	assert.Equal(t, ".policy.yml", got.Path)
	assert.True(t, got.IsRemote)
	assert.Equal(t, []byte("policy:\n  approval: []\n"), got.Content)
}

func TestCrossOrgConfigLoader_CrossOrgRemoteResolvesInstallation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/orgs/target-org", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":"target-org"}`))
	})
	requestingClient := newTestGitHubClient(t, mux)

	remoteOrgClient := newTestGitHubClient(t, http.NewServeMux())

	var capturedClient *github.Client
	var capturedOwner, capturedRepo, capturedRef string

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				return appconfig.Config{
					Content: []byte("remote: target-org/shared-policy\npath: policies/.policy.yml\nref: main\n"),
					Source:  "testorg/testrepo@main",
					Path:    ".policy.yml",
				}, nil
			},
		},
		paths: []string{".policy.yml"},
		clientCreator: fakeClientCreator{
			byInstallationID: map[int64]*github.Client{42: remoteOrgClient},
		},
		installations: fakeInstallations{
			byOwner: map[string]githubapp.Installation{
				"target-org": {ID: 42, Owner: "target-org"},
			},
		},
		newRemoteLoader: func(paths []string) ConfigLoader {
			require.Equal(t, []string{"policies/.policy.yml"}, paths)
			return mockConfigLoader{
				loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
					capturedClient = client
					capturedOwner, capturedRepo, capturedRef = owner, repo, ref
					return appconfig.Config{
						Content: []byte("policy:\n  approval: []\n"),
					}, nil
				},
			}
		},
	}

	got, err := loader.LoadConfig(context.Background(), requestingClient, "testorg", "testrepo", "main")
	require.NoError(t, err)

	assert.Same(t, remoteOrgClient, capturedClient, "cross-org remote must use the client scoped to the target org's installation")
	assert.Equal(t, "target-org", capturedOwner)
	assert.Equal(t, "shared-policy", capturedRepo)
	assert.Equal(t, "main", capturedRef)

	assert.Equal(t, "target-org/shared-policy@main", got.Source)
	assert.Equal(t, "policies/.policy.yml", got.Path)
	assert.True(t, got.IsRemote)
}

func TestCrossOrgConfigLoader_CrossOrgNoInstallationReturnsClearError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/orgs/target-org", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":"target-org"}`))
	})
	requestingClient := newTestGitHubClient(t, mux)

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				return appconfig.Config{
					Content: []byte("remote: target-org/shared-policy\nref: main\n"),
					Source:  "testorg/testrepo@main",
					Path:    ".policy.yml",
				}, nil
			},
		},
		paths:         []string{".policy.yml"},
		clientCreator: fakeClientCreator{},
		installations: fakeInstallations{}, // no entry for "target-org"
		newRemoteLoader: func(paths []string) ConfigLoader {
			t.Fatal("newRemoteLoader should not be called when installation resolution fails")
			return nil
		},
	}

	_, err := loader.LoadConfig(context.Background(), requestingClient, "testorg", "testrepo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `policy-bot is not installed on organization "target-org"`)
}

func TestCrossOrgConfigLoader_RemoteFileNotFound(t *testing.T) {
	requestingClient := newTestGitHubClient(t, http.NewServeMux())

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				return appconfig.Config{
					Content: []byte("remote: testorg/shared-policy\nref: main\n"),
					Source:  "testorg/testrepo@main",
					Path:    ".policy.yml",
				}, nil
			},
		},
		paths:         []string{".policy.yml"},
		clientCreator: fakeClientCreator{},
		installations: fakeInstallations{},
		newRemoteLoader: func(paths []string) ConfigLoader {
			return mockConfigLoader{
				loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
					return appconfig.Config{}, nil // undefined: file does not exist
				},
			}
		},
	}

	_, err := loader.LoadConfig(context.Background(), requestingClient, "testorg", "testrepo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid remote reference: file does not exist")
}

func TestCrossOrgConfigLoader_DefaultsPathAndRef(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/testorg/shared-policy", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"default_branch":"trunk"}`))
	})
	requestingClient := newTestGitHubClient(t, mux)

	var capturedRepo, capturedRef string

	loader := &CrossOrgConfigLoader{
		inner: mockConfigLoader{
			loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
				// No "path" or "ref" key: both should fall back to defaults.
				return appconfig.Config{
					Content: []byte("remote: testorg/shared-policy\n"),
					Source:  "testorg/testrepo@main",
					Path:    ".policy.yml",
				}, nil
			},
		},
		paths:         []string{".policy.yml"},
		clientCreator: fakeClientCreator{},
		installations: fakeInstallations{},
		newRemoteLoader: func(paths []string) ConfigLoader {
			require.Equal(t, []string{".policy.yml"}, paths, "empty remote path should default to the first configured path")
			return mockConfigLoader{
				loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
					capturedRepo, capturedRef = repo, ref
					return appconfig.Config{Content: []byte("policy:\n  approval: []\n")}, nil
				},
			}
		},
	}

	_, err := loader.LoadConfig(context.Background(), requestingClient, "testorg", "testrepo", "main")
	require.NoError(t, err)

	assert.Equal(t, "shared-policy", capturedRepo)
	assert.Equal(t, "trunk", capturedRef, "empty remote ref should default to the remote repository's default branch")
}

// serveFileContent registers a handler on mux that responds like the GitHub
// contents API for a single file, base64-encoding content the way GitHub
// really does.
func serveFileContent(t *testing.T, mux *http.ServeMux, pattern, content string) {
	t.Helper()

	body, err := json.Marshal(github.RepositoryContent{
		Type:     github.Ptr("file"),
		Encoding: github.Ptr("base64"),
		Content:  github.Ptr(base64.StdEncoding.EncodeToString([]byte(content))),
	})
	require.NoError(t, err)

	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}

// TestCrossOrgConfigLoader_RealRemoteLoaderRejectsMissingRemoteFile is a
// regression test for the production newRemoteLoader built by
// NewCrossOrgConfigLoader. It does not mock newRemoteLoader (unlike every
// other test in this file): it builds the loader through the real
// constructor and drives it against an httptest-backed GitHub API, so it
// exercises the actual appconfig.Loader used in production.
//
// Without appconfig.WithOwnerDefault("", nil) on that loader, a missing
// remote file falls through to the target org's ".github" repository
// default policy instead of producing the documented
// "invalid remote reference: file does not exist" error. This test proves
// that fallback is not taken: the ".github" repo below serves real content
// that would make the test wrongly pass if the org-default fallback fired.
func TestCrossOrgConfigLoader_RealRemoteLoaderRejectsMissingRemoteFile(t *testing.T) {
	mux := http.NewServeMux()

	// The local policy file: a remote reference with an explicit ref so we
	// don't need to mock the remote repository's default-branch lookup.
	serveFileContent(t, mux, "/repos/testorg/testrepo/contents/.policy.yml",
		"remote: testorg/shared-policy\nref: main\n")

	// The remote file genuinely does not exist.
	mux.HandleFunc("/repos/testorg/shared-policy/contents/.policy.yml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	// testorg's ".github" repo default policy DOES exist. If the buggy
	// org-default fallback were taken, this content would be returned
	// (mislabeled as the shared-policy remote) instead of an error.
	// loadDefaultConfig first looks up the ".github" repo itself (for its
	// default branch), then fetches the file from it.
	mux.HandleFunc("/repos/testorg/.github", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":".github","default_branch":"main"}`))
	})
	serveFileContent(t, mux, "/repos/testorg/.github/contents/.policy.yml",
		"policy:\n  approval: []\n")

	client := newTestGitHubClient(t, mux)

	loader := NewCrossOrgConfigLoader([]string{".policy.yml"}, fakeClientCreator{}, fakeInstallations{})

	cfg, err := loader.LoadConfig(context.Background(), client, "testorg", "testrepo", "main")
	require.Error(t, err, "got config %+v", cfg)
	assert.Contains(t, err.Error(), "invalid remote reference: file does not exist")
}
