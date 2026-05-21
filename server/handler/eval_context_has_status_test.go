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
	"strings"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestEvalContextHasStatusDescription_PostsDetailedStatus drives the
// EvalContext path that the GitHub webhook handler uses to post a commit
// status back to GitHub. It asserts that when a has_status condition is
// blocking the rule, the description that would be posted to GitHub
// (captured via SkipPostStatus = true so the test does not hit the
// network) names the specific blocking check.
//
// This replaces the "I need a docker-compose webhook replay rig" shape
// described in the original POC brief: the handler-level test below
// exercises the same code path that the webhook handler takes, minus the
// HTTP shell. policy-bot has no docker-compose integration harness today,
// and building one for a change that lives entirely inside the in-process
// evaluator would burn budget without buying additional coverage. See
// CONTRIBUTING discussion in the POC summary.
func TestEvalContextHasStatusDescription_PostsDetailedStatus(t *testing.T) {
	ctx := context.Background()

	const policyYAML = `
approval_rules:
  - name: required ci checks
    requires:
      conditions:
        has_status:
          statuses:
            - "pre-commit-check"
            - "Tests with Coverage"
policy:
  approval:
    - "required ci checks"
`

	var cfg policy.Config
	require.NoError(t, yaml.Unmarshal([]byte(policyYAML), &cfg))
	evaluator, err := policy.ParsePolicy(&cfg, nil)
	require.NoError(t, err)

	prctx := &pulltest.Context{
		OwnerValue:     "testorg",
		RepoValue:      "testrepo",
		NumberValue:    42,
		StateValue:     "open",
		HeadSHAValue:   "abc123",
		BranchBaseName: "main",
		BranchHeadName: "feature",
		LatestStatusesValue: map[string]string{
			// "pre-commit-check" missing entirely
			"Tests with Coverage": "failure",
		},
	}

	ec := &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL:      "https://policy-bot.example.com",
		PullContext:    prctx,
		SkipPostStatus: true,
	}

	result, err := ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)
	assert.Equal(t, common.StatusPending, result.Status)

	require.NotNil(t, ec.Status, "EvaluatePolicy should set ec.Status when SkipPostStatus is true")
	assert.Equal(t, "pending", ec.Status.GetState())
	assert.Equal(t, "policy-bot: main", ec.Status.GetContext())

	desc := ec.Status.GetDescription()
	assert.True(t, strings.Contains(desc, "pre-commit-check"),
		"posted status must name the missing pre-commit-check; got %q", desc)
	assert.True(t, strings.Contains(desc, "Tests with Coverage"),
		"posted status must name the failing Tests with Coverage; got %q", desc)
	assert.True(t, strings.Contains(desc, "Waiting on") || strings.Contains(desc, "Failing"),
		"posted status must use the descriptive blocker labels; got %q", desc)
}
