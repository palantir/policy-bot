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

package handler

import (
	"context"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticEvaluator common.Result

func (eval *staticEvaluator) Trigger() common.Trigger {
	return common.TriggerStatic
}

func (eval *staticEvaluator) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return common.Result(*eval)
}

func TestEvalContext_EvaluatePolicy_PendingAsFailure(t *testing.T) {
	ctx := context.Background()

	tests := map[string]struct {
		serverPendingAsFailure bool
		policyPendingAsFailure *bool
		resultStatus           common.EvaluationStatus
		expectedStatus         string
		expectedConclusion     string
	}{
		"default_pending_returns_in_progress": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusPending,
			expectedStatus:         "in_progress",
			expectedConclusion:     "",
		},
		"server_option_enabled_returns_failure": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusPending,
			expectedStatus:         "completed",
			expectedConclusion:     "failure",
		},
		"policy_option_enabled_returns_failure": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: ptr(true),
			resultStatus:           common.StatusPending,
			expectedStatus:         "completed",
			expectedConclusion:     "failure",
		},
		"policy_option_overrides_server_to_disable": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: ptr(false),
			resultStatus:           common.StatusPending,
			expectedStatus:         "in_progress",
			expectedConclusion:     "",
		},
		"approved_returns_success_regardless": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusApproved,
			expectedStatus:         "completed",
			expectedConclusion:     "success",
		},
		"disapproved_returns_failure_regardless": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusDisapproved,
			expectedStatus:         "completed",
			expectedConclusion:     "failure",
		},
		"skipped_returns_action_required_regardless": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusSkipped,
			expectedStatus:         "completed",
			expectedConclusion:     "action_required",
		},
		"both_server_and_policy_enabled": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: ptr(true),
			resultStatus:           common.StatusPending,
			expectedStatus:         "completed",
			expectedConclusion:     "failure",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			prctx := &pulltest.Context{
				OwnerValue:     "testowner",
				RepoValue:      "testrepo",
				NumberValue:    1,
				HeadSHAValue:   "abc123",
				BranchBaseName: "main",
			}

			policyConfig := &policy.Config{}
			policyConfig.PendingAsFailure = test.policyPendingAsFailure

			ec := &EvalContext{
				Options: &PullEvaluationOptions{
					StatusCheckContext: "policy-bot",
					PendingAsFailure:   test.serverPendingAsFailure,
				},
				PublicURL:   "http://localhost:8080",
				PullContext: prctx,
				Config: FetchedConfig{
					Config: policyConfig,
					Source: "test",
					Path:   ".policy.yml",
				},
				SkipPostStatus: true,
			}

			evaluator := &staticEvaluator{
				Status:            test.resultStatus,
				StatusDescription: "test description",
			}

			_, err := ec.EvaluatePolicy(ctx, evaluator)
			require.NoError(t, err)

			require.NotNil(t, ec.Status, "status should be set")
			assert.Equal(t, test.expectedStatus, ec.Status.GetStatus(), "unexpected check run status")
			assert.Equal(t, test.expectedConclusion, ec.Status.GetConclusion(), "unexpected check run conclusion")
		})
	}
}

func TestEvalContext_PostStatus_CheckRunFields(t *testing.T) {
	ctx := context.Background()

	prctx := &pulltest.Context{
		OwnerValue:     "testowner",
		RepoValue:      "testrepo",
		NumberValue:    42,
		HeadSHAValue:   "def456",
		BranchBaseName: "main",
	}

	ec := &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL:      "http://localhost:8080",
		PullContext:    prctx,
		SkipPostStatus: true,
	}

	ec.PostStatus(ctx, "success", "Approved by alice")

	require.NotNil(t, ec.Status)
	assert.Equal(t, "policy-bot: main", ec.Status.Name, "check run name should include branch")
	assert.Equal(t, "def456", ec.Status.HeadSHA)
	assert.Equal(t, "http://localhost:8080/details/testowner/testrepo/42", ec.Status.GetDetailsURL())
	require.NotNil(t, ec.Status.Output)
	assert.Equal(t, "Approved by alice", ec.Status.Output.GetTitle())
	assert.Equal(t, "Approved by alice", ec.Status.Output.GetSummary())
}

func TestEvalContext_EvaluatePolicy_PendingAsFailure_NilConfig(t *testing.T) {
	ctx := context.Background()

	prctx := &pulltest.Context{
		OwnerValue:     "testowner",
		RepoValue:      "testrepo",
		NumberValue:    1,
		HeadSHAValue:   "abc123",
		BranchBaseName: "main",
	}

	// Test with nil Config in FetchedConfig
	ec := &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
			PendingAsFailure:   true,
		},
		PublicURL:   "http://localhost:8080",
		PullContext: prctx,
		Config: FetchedConfig{
			Config: nil, // nil config
			Source: "test",
			Path:   ".policy.yml",
		},
		SkipPostStatus: true,
	}

	evaluator := &staticEvaluator{
		Status:            common.StatusPending,
		StatusDescription: "test description",
	}

	_, err := ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)

	require.NotNil(t, ec.Status, "status should be set")
	assert.Equal(t, "completed", ec.Status.GetStatus(), "server option should be used when policy config is nil")
	assert.Equal(t, "failure", ec.Status.GetConclusion(), "server option should be used when policy config is nil")
}
