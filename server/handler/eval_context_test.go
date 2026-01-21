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
		expectedState          string
	}{
		"default_pending_returns_pending": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusPending,
			expectedState:          "pending",
		},
		"server_option_enabled_returns_failure": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusPending,
			expectedState:          "failure",
		},
		"policy_option_enabled_returns_failure": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: ptr(true),
			resultStatus:           common.StatusPending,
			expectedState:          "failure",
		},
		"policy_option_overrides_server_to_disable": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: ptr(false),
			resultStatus:           common.StatusPending,
			expectedState:          "pending",
		},
		"approved_returns_success_regardless": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusApproved,
			expectedState:          "success",
		},
		"disapproved_returns_failure_regardless": {
			serverPendingAsFailure: false,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusDisapproved,
			expectedState:          "failure",
		},
		"skipped_returns_error_regardless": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: nil,
			resultStatus:           common.StatusSkipped,
			expectedState:          "error",
		},
		"both_server_and_policy_enabled": {
			serverPendingAsFailure: true,
			policyPendingAsFailure: ptr(true),
			resultStatus:           common.StatusPending,
			expectedState:          "failure",
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
			assert.Equal(t, test.expectedState, *ec.Status.State, "expected state %q, got %q", test.expectedState, *ec.Status.State)
		})
	}
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
	assert.Equal(t, "failure", *ec.Status.State, "server option should be used when policy config is nil")
}
