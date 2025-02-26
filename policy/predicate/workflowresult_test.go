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
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

type WorkflowTestCase struct {
	name                    string
	latestWorkflowRunsValue map[string][]*github.WorkflowRun
	latestWorkflowRunsError error
	predicate               Predicate
	ExpectedPredicateResult *common.PredicateResult
}

func mockWorkflowRun(status string, conclusion string) *github.WorkflowRun {
	id := int64(1)
	name := "abc"
	nodeID := "MDg6V29ya2Zsb3cx"
	headBranch := "main"
	headSHA := "abc123"
	path := ".github/workflows/test.yml"
	runNumber := 1
	runAttempt := 1
	event := "push"
	displayTitle := "Test Workflow"
	workflowID := int64(123456)
	checkSuiteID := int64(654321)
	checkSuiteNodeID := "MDg6Q2hlY2tTdWl0ZQ=="
	url := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1"
	htmlURL := "https://github.com/octocat/Hello-World/actions/runs/1"
	createdAt := github.Timestamp{Time: time.Now()}
	updatedAt := github.Timestamp{Time: time.Now()}
	runStartedAt := github.Timestamp{Time: time.Now()}
	jobsURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/jobs"
	logsURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/logs"
	checkSuiteURL := "https://api.github.com/repos/octocat/Hello-World/check-suites/654321"
	artifactsURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/artifacts"
	cancelURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/cancel"
	rerunURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/rerun"
	previousAttemptURL := "https://api.github.com/repos/octocat/Hello-World/actions/runs/1/attempts/1"
	headCommit := &github.HeadCommit{
		ID:      github.String("abc123"),
		Message: github.String("Initial commit"),
	}
	repository := &github.Repository{
		ID:   github.Int64(1296269),
		Name: github.String("Hello-World"),
	}
	actor := &github.User{
		Login: github.String("octocat"),
		ID:    github.Int64(1),
	}

	return &github.WorkflowRun{
		ID:                 &id,
		Name:               &name,
		NodeID:             &nodeID,
		HeadBranch:         &headBranch,
		HeadSHA:            &headSHA,
		Path:               &path,
		RunNumber:          &runNumber,
		RunAttempt:         &runAttempt,
		Event:              &event,
		DisplayTitle:       &displayTitle,
		Status:             &status,
		Conclusion:         &conclusion,
		WorkflowID:         &workflowID,
		CheckSuiteID:       &checkSuiteID,
		CheckSuiteNodeID:   &checkSuiteNodeID,
		URL:                &url,
		HTMLURL:            &htmlURL,
		CreatedAt:          &createdAt,
		UpdatedAt:          &updatedAt,
		RunStartedAt:       &runStartedAt,
		JobsURL:            &jobsURL,
		LogsURL:            &logsURL,
		CheckSuiteURL:      &checkSuiteURL,
		ArtifactsURL:       &artifactsURL,
		CancelURL:          &cancelURL,
		RerunURL:           &rerunURL,
		PreviousAttemptURL: &previousAttemptURL,
		HeadCommit:         headCommit,
		Repository:         repository,
		Actor:              actor,
	}
}

func TestHasSuccessfulWorkflowResultRun(t *testing.T) {
	commonTestCases := []WorkflowTestCase{
		{
			name: "all workflows succeed",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "success")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "failure")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success"), mockWorkflowRun("completed", "failure")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "failure")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "failure")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "failure")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "skipped")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "skipped")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "skipped")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success"), mockWorkflowRun("completed", "skipped")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "failure")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "success")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "skipped")},
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
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success"), mockWorkflowRun("completed", "skipped")},
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

	runWorkflowResultTestCase(t, commonTestCases)
}

func runWorkflowResultTestCase(t *testing.T, cases []WorkflowTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := tc.predicate.Evaluate(ctx, &pulltest.Context{
				LatestWorkflowRunsValue: tc.latestWorkflowRunsValue,
				LatestWorkflowRunsError: tc.latestWorkflowRunsError,
			})
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
