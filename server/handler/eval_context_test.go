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

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
	assert.Equal(t, "Error loading policy from testorg/testrepo@main", ec.Status.GetDescription())
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
			Source:     "testorg/testrepo@main",
			Path:       ".policy.yml",
			SeenPolicy: seenPolicy,
		},
	}
}

func TestWorkflowCompletionPolicyStatus(t *testing.T) {
	for name, tt := range map[string]struct{ conclusion, want string }{
		"successful workflow": {"success", "success"},
		"failed workflow":     {"failure", "pending"},
	} {
		t.Run(name, func(t *testing.T) {
			var config policy.Config
			require.NoError(t, yaml.UnmarshalStrict([]byte(`policy:
  approval: [CI passes]
approval_rules:
  - name: CI passes
    requires:
      conditions:
        has_workflow_result:
          workflows: [.github/workflows/ci.yml]
`), &config))
			ec := &EvalContext{
				Options:   &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
				PublicURL: "https://policy.example.com",
				PullContext: &pulltest.Context{
					OwnerValue: "owner", RepoValue: "repo", NumberValue: 42,
					StateValue: "open", HeadSHAValue: "head", BranchBaseName: "main",
					LatestWorkflowRunsValue: map[string][]string{".github/workflows/ci.yml": {tt.conclusion}},
				},
				Config:         FetchedConfig{Config: &config, Source: "owner/repo@main", Path: ".policy.yml"},
				SkipPostStatus: true,
			}
			require.NoError(t, ec.Evaluate(context.Background(), common.TriggerWorkflowRun))
			if assert.NotNil(t, ec.Status, "workflow completion should produce a policy status") {
				assert.Equal(t, tt.want, ec.Status.GetState(), "workflow result determines policy status")
				assert.Equal(t, "policy-bot: main", ec.Status.GetContext(), "policy status context")
			}
		})
	}
}
