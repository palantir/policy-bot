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
	"errors"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigSuppressesStatusForUnseenPolicy(t *testing.T) {
	ec := makeEvalContext(false)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	assert.Nil(t, ec.Status)
}

func TestParseConfigPostsStatusForSeenPolicy(t *testing.T) {
	ec := makeEvalContext(true)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	require.NotNil(t, ec.Status)
	assert.Equal(t, "error", ec.Status.GetState())
	assert.Equal(t, "policy-bot: main", ec.Status.GetContext())
	assert.Contains(t, ec.Status.GetDescription(), "testorg/testrepo@main")
}

func TestExtensionPolicyErrorSources(t *testing.T) {
	const parentPolicy = "policy: {approval: [{slot: {name: service, required: true}}]}"
	for _, test := range []struct {
		name, child, parent, message string
	}{
		{"required slot", extendingPolicy, parentPolicy, "required slots are empty"},
		{"standalone", "policy: {approval: [missing]}", "", "failed to parse approval policy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fetcher := ConfigFetcher{Loader: mockConfigLoader{
				loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(test.child), Source: "example/service@main", Path: "child.yml"}, nil
				},
				loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
					require.NotEmpty(t, test.parent, "only an extending policy may request a parent")
					return appconfig.Config{Content: []byte(test.parent), Source: "shared/policies@release", Path: "rules/parent.yml"}, nil
				},
			}}
			ec := makeEvalContext(false)
			ec.Config = fetcher.ConfigForRepositoryBranch(context.Background(), nil, "example", "service", "main")
			parsed, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			require.ErrorContains(t, err, test.message, "validation failures must reach the handler")
			assert.Nil(t, parsed, "invalid policies must not be evaluated")
			assert.Contains(t, err.Error(), "example/service@main", "diagnostic identifies the child repository")
			assert.Contains(t, err.Error(), "child.yml", "diagnostic identifies the child file")
			require.NotNil(t, ec.Status, "validation failures must post a status")
			assert.Equal(t, "error", ec.Status.GetState(), "invalid policies post an error status")
			assert.Contains(t, ec.Status.GetDescription(), "example/service@main", "status identifies the child repository")
			assert.Contains(t, ec.Status.GetDescription(), "child.yml", "status identifies the child file")
			if test.parent != "" {
				assert.Contains(t, err.Error(), "shared/policies@release", "diagnostic identifies the parent repository")
				assert.Contains(t, err.Error(), "rules/parent.yml", "diagnostic identifies the parent file")
			}
		})
	}
}

func makeEvalContext(seenPolicy bool) *EvalContext {
	return &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL: "https://policy-bot.example.com",
		PullContext: &pulltest.Context{
			OwnerValue:     "testorg",
			RepoValue:      "testrepo",
			NumberValue:    42,
			StateValue:     "open",
			HeadSHAValue:   "abc123",
			BranchBaseName: "main",
			BranchHeadName: "feature",
		},
		SkipPostStatus: true,
		Config: FetchedConfig{
			LoadError:  errors.New("request failed"),
			Sources:    []PolicyLocation{{Source: "testorg/testrepo@main", Path: ".policy.yml"}},
			SeenPolicy: seenPolicy,
		},
	}
}
