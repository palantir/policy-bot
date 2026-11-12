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
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const extendingPolicy = "policy: {approval_extends: {remote: shared/policies, path: parent.yml, ref: main}}"

func TestConfigFetcherForcedSharedParentPath(t *testing.T) {
	const policyPath = "config/approval.yml"
	for _, test := range []struct {
		name, parentPath, wantPath string
	}{
		{"configured default", "", policyPath},
		{"explicit path", "custom.yml", "custom.yml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := "remote: shared/policies"
			if test.parentPath != "" {
				remote += ", path: " + test.parentPath
			}
			child := "policy: {approval_extends: {" + remote + "}}" + childSlotFill
			client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var content string
				switch r.Method + " " + r.URL.Path {
				case "GET /repos/original/.github":
					assert.NoError(t, json.NewEncoder(w).Encode(github.Repository{Name: new(".github"), DefaultBranch: new("org-main")}))
					return
				case "GET /repos/original/.github/contents/policy.yml":
					assert.Equal(t, "org-main", r.URL.Query().Get("ref"))
					content = child
				case "GET /repos/shared/policies":
					assert.NoError(t, json.NewEncoder(w).Encode(github.Repository{Name: new("policies"), DefaultBranch: new("parent-main")}))
					return
				case "GET /repos/shared/policies/contents/" + test.wantPath:
					assert.Equal(t, "parent-main", r.URL.Query().Get("ref"))
					content = slotPolicy
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				assert.NoError(t, json.NewEncoder(w).Encode(github.RepositoryContent{
					Type: new("file"), Encoding: new("base64"),
					Content: new(base64.StdEncoding.EncodeToString([]byte(content))),
				}))
			})
			fetcher := ConfigFetcher{
				Loader:            appconfig.NewLoader(nil, appconfig.WithOwnerDefault(".github", []string{"policy.yml"})),
				DefaultParentPath: policyPath,
				SeenPolicyCache:   NewSeenPolicyCache(),
			}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), client, "original", "service", "release")
			require.NoError(t, fc.LoadError)
			require.NoError(t, fc.ParseError)
			require.NotNil(t, fc.Config)
			require.NotNil(t, fc.Parent)
			assert.Equal(t, "original/.github@org-main", fc.Source)
			assert.Equal(t, "policy.yml", fc.Path)
			assert.Equal(t, test.parentPath, fc.Config.Policy.ApprovalExtends.Path)
			parsed, err := policy.ParsePolicyWithParent(fc.Config, fc.Parent, nil)
			require.NoError(t, err)
			assert.NotNil(t, parsed.FindApprovalRule("service", "Child"))
		})
	}
}

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
		{name: "remote pointer", content: "remote: shared/third", parseError: "field remote not found"},
		{name: "merged remote pointer", content: "<<: {remote: shared/third}", parseError: "field remote not found"},
		{name: "null remote pointer", content: "remote: null", parseError: "field remote not found"},
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
			if test.undefined {
				assert.Empty(t, fc.ParentSource)
				assert.Empty(t, fc.ParentPath)
			} else {
				assert.Equal(t, "shared/policies@main", fc.ParentSource)
				assert.Equal(t, "parent.yml", fc.ParentPath)
			}
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
	fetcher := ConfigFetcher{Loader: mockConfigLoader{
		loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
			return appconfig.Config{
				Content: []byte("policy: {approval: [], approval_extends: {remote: shared/policies}}"),
				Source:  "original/service@release",
				Path:    ".policy.yml",
			}, nil
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
}

func TestConfigFetcherParentRetry(t *testing.T) {
	for _, test := range []struct {
		name        string
		successCall int
		cancel      bool
		wantCalls   int
	}{
		{name: "transient error", successCall: 2, wantCalls: 2},
		{name: "bounded retries", wantCalls: 4},
		{name: "cancel backoff", cancel: true, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls, childCalls := 0, 0
				failure := fmt.Errorf("read file: %w", githubErr(http.StatusBadGateway))
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

func TestConfigFetcherLoadsParent(t *testing.T) {
	for _, test := range []struct {
		name, source, parent string
	}{
		{"local child", "original/service@release", "policy: {approval: [review]}\napproval_rules: [{name: review}]"},
		{"remote child", "shared/policies@main", "policy: {approval: [review]}\napproval_rules: [{name: review}]"},
		{"empty parent", "shared/policies@main", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := github.NewClient()
			require.NoError(t, err)
			parentCalls := 0
			fetcher := ConfigFetcher{Loader: mockConfigLoader{
				loadConfig: func(gotCtx context.Context, gotClient *github.Client, owner, repo, ref string) (appconfig.Config, error) {
					assert.Same(t, ctx, gotCtx)
					assert.Same(t, client, gotClient)
					assert.Equal(t, "original", owner)
					assert.Equal(t, "service", repo)
					assert.Equal(t, "release", ref)
					return appconfig.Config{Content: []byte(extendingPolicy), Source: test.source, Path: "child.yml"}, nil
				},
				loadRemoteRef: func(gotCtx context.Context, gotClient *github.Client, owner string, remote appconfig.RemoteRef) (appconfig.Config, error) {
					parentCalls++
					assert.Same(t, ctx, gotCtx)
					assert.Same(t, client, gotClient)
					assert.Equal(t, "original", owner)
					assert.Equal(t, appconfig.RemoteRef{Remote: "shared/policies", Path: "parent.yml", Ref: "main"}, remote)
					return appconfig.Config{Content: []byte(test.parent), Source: "shared/policies@main", Path: "parent.yml"}, nil
				},
			}}
			fc := fetcher.ConfigForRepositoryBranch(ctx, client, "original", "service", "release")
			require.NoError(t, fc.LoadError)
			require.NoError(t, fc.ParseError)
			require.NotNil(t, fc.Config)
			require.NotNil(t, fc.Parent)
			assert.NotNil(t, fc.Config.Policy.ApprovalExtends)
			if test.parent == "" {
				assert.Empty(t, fc.Parent.Policy.Approval)
			} else {
				assert.Equal(t, approval.Policy{"review"}, fc.Parent.Policy.Approval)
			}
			assert.Equal(t, 1, parentCalls)
			assert.Equal(t, test.source, fc.Source)
			assert.Equal(t, "child.yml", fc.Path)
			assert.True(t, fc.SeenPolicy)
		})
	}
}
