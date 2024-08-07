// Copyright 2024 Palantir Technologies, Inc.
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

package predicate

import (
	"context"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

type WorkflowTestCase struct {
	name                    string
	latestWorkflowRunsValue map[string][]string
	latestWorkflowRunsError error
	predicate               Predicate
	ExpectedPredicateResult *common.PredicateResult
}

func TestHasSuccessfulWorkflowRun(t *testing.T) {
	commonTestCases := []WorkflowTestCase{
		{
			name: "all workflows succeed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"success"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "multiple workflows succeed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"success"},
				".github/workflows/test2.yml": {"success"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow fails",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"failure"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow fails and succeeds",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"failure", "success"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "multiple workflows fail",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"failure"},
				".github/workflows/test2.yml": {"failure"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
		},
		{
			name: "one success, one failure",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"success"},
				".github/workflows/test2.yml": {"failure"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name:                    "a workflow is missing",
			latestWorkflowRunsValue: map[string][]string{},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name:                    "multiple workflow are missing",
			latestWorkflowRunsValue: map[string][]string{},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow is missing, the other workflow is skipped",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test2.yml": {"skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows: []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow is skipped, but skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml"},
				Conclusions: AllowedConclusions{"skipped"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow succeeds, the other workflow is skipped, but skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"success"},
				".github/workflows/test2.yml": {"skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
				Conclusions: AllowedConclusions{"skipped", "success"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow succeeds and is skipped, but skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"success", "skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml"},
				Conclusions: AllowedConclusions{"skipped", "success"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow fails, the other workflow is skipped, but skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"failure"},
				".github/workflows/test2.yml": {"skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
				Conclusions: AllowedConclusions{"skipped", "success"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow succeeds, the other workflow is skipped, only skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml":  {"success"},
				".github/workflows/test2.yml": {"skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
				Conclusions: AllowedConclusions{"skipped"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow succeeds and is skipped, only skipped workflows are allowed",
			latestWorkflowRunsValue: map[string][]string{
				".github/workflows/test.yml": {"success", "skipped"},
			},
			predicate: HasWorkflowResult{
				Workflows:   []string{".github/workflows/test.yml"},
				Conclusions: AllowedConclusions{"skipped"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
	}

	runWorkflowTestCase(t, commonTestCases)
}

func runWorkflowTestCase(t *testing.T, cases []WorkflowTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := tc.predicate.Evaluate(ctx, &pulltest.Context{
				LatestWorkflowRunsValue: tc.latestWorkflowRunsValue,
				LatestStatusesError:     tc.latestWorkflowRunsError,
			})
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
