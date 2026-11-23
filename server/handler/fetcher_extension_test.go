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
	"net/http"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const extendingPolicy = "policy: {approval_extends: {remote: shared/policies, path: parent.yml, ref: main}}"

func TestConfigFetcherParentFailures(t *testing.T) {
	denied := githubErr(http.StatusForbidden)
	for _, test := range []struct {
		name, content, parseError string
		loadError                 error
	}{
		{name: "denied", loadError: denied},
		{name: "remote pointer", content: "remote: shared/third", parseError: "field remote not found"},
		{name: "further extension", content: extendingPolicy, parseError: "must not extend another policy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			fetcher := ConfigFetcher{Loader: mockConfigLoader{
				loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(extendingPolicy), Source: "shared/policies@main", Path: "child.yml"}, nil
				},
				loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
					calls++
					return appconfig.Config{Content: []byte(test.content), Source: "shared/policies@main", Path: "parent.yml"}, test.loadError
				},
			}}
			fc := fetcher.ConfigForRepositoryBranch(context.Background(), nil, "original", "service", "release")
			require.NotNil(t, fc.Config, "the decoded child must survive a parent failure")
			if test.loadError != nil {
				require.ErrorIs(t, fc.LoadError, test.loadError)
				assert.True(t, fc.SeenPolicy, "a parent load failure must not suppress the policy error status")
			} else {
				require.ErrorContains(t, fc.ParseError, test.parseError)
			}
			if test.name == "further extension" {
				assert.Equal(t, 1, calls, "the parent must not trigger another fetch")
			}
		})
	}
}

func TestConfigFetcherLoadsParent(t *testing.T) {
	opts := &policy.GlobalOptions{IgnoreEditedComments: new(true)}
	fetcher := ConfigFetcher{Options: opts, Loader: mockConfigLoader{
		loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
			return appconfig.Config{Content: []byte(extendingPolicy + `
slots: {service: [Review]}
approval_rules: [{name: Review, options: {ignore_edited_comments: false}}]
`), Source: "shared/policies@main", Path: "child.yml"}, nil
		},
		loadRemoteRef: func(_ context.Context, _ *github.Client, owner string, remote appconfig.RemoteRef) (appconfig.Config, error) {
			assert.Equal(t, "original", owner)
			assert.Equal(t, appconfig.RemoteRef{Remote: "shared/policies", Path: "parent.yml", Ref: "main"}, remote)
			return appconfig.Config{Content: []byte(`
policy: {approval: [Review, {slot: {name: service, required: true}}]}
approval_rules: [{name: Review, options: {ignore_edited_comments: false}}]
`), Source: "shared/policies@main", Path: "parent.yml"}, nil
		},
	}}
	fc := fetcher.ConfigForRepositoryBranch(context.Background(), nil, "original", "service", "release")
	require.NoError(t, fc.LoadError)
	require.NoError(t, fc.ParseError)
	require.NotNil(t, fc.Policy)
	assert.Equal(t, []PolicyLocation{{Source: "shared/policies@main", Path: "child.yml"}, {Source: "shared/policies@main", Path: "parent.yml"}}, fc.Sources)
	for _, slot := range []string{"", "service"} {
		rule := fc.Policy.FindApprovalRule(slot, "Review")
		require.NotNil(t, rule)
		assert.True(t, rule.Options.IsIgnoreEditedComments())
	}
}
