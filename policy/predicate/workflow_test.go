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

	"github.com/google/go-github/v70/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestHasSuccessfulWorkflowRun(t *testing.T) {
	commonTestCases := []WorkflowTestCase{
		{
			name: "all workflows succeed",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
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
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
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
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow fails and succeeds",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "failure"), mockWorkflowRun("completed", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
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
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
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
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name:                    "a workflow is missing",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name:                    "multiple workflow are missing",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
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
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
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
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
				Conclusions: []string{"skipped"},
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
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
				Conclusions: []string{"skipped", "success"},
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
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
				Conclusions: []string{"skipped", "success"},
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
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "skipped")},
			},
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
				Conclusions: []string{"skipped", "success"},
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
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml"), common.NewMustCompileRegexp(".github/workflows/test2.yml")},
				Conclusions: []string{"skipped"},
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
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
				Conclusions: []string{"skipped"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'expected', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("expected", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'failure', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'in_progress', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("in_progress", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'queued', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("queued", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'pending', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("pending", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'waiting', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("waiting", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'requested', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("requested", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'startup_failure', it should be ignored by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("startup_failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".github/workflows/test.yml")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a workflow in state 'expected' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("expected", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"expected"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'failure' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'in_progress' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("in_progress", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"in_progress"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'queued' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("queued", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"queued"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'pending' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("pending", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"pending"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'waiting' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("waiting", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"waiting"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'requested' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("requested", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"requested"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a workflow in state 'startup_failure' and this state is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test2.yml": {mockWorkflowRun("startup_failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"startup_failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "workflows in state 'expected' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("expected", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"expected"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'failure' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'in_progress' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("in_progress", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"in_progress"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'queued' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("queued", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"queued"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'pending' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("pending", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"pending"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'waiting' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("waiting", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"waiting"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'requested' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("requested", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"requested"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "workflows in state 'startup_failure' and 'completed' but only 'expected' is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test1.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("startup_failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"startup_failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test1.yml"},
			},
		},
		{
			name: "a non existing Status is required, it should not match",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"complete"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a non existing conclusion is required, it should not match",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml": {mockWorkflowRun("completed", "success")},
			},
			predicate: HasWorkflow{
				Workflows:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"succes"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test.yml"},
			},
		},
		{
			name: "a only state completed is allowed, it should only match the allowed state",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("requested", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a any state is allowed, it should match all the states and conclusion 'success' by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("expected", "")},
				".github/workflows/test3.yml": {mockWorkflowRun("failure", "")},
				".github/workflows/test4.yml": {mockWorkflowRun("in_progress", "")},
				".github/workflows/test5.yml": {mockWorkflowRun("queued", "")},
				".github/workflows/test6.yml": {mockWorkflowRun("pending", "")},
				".github/workflows/test7.yml": {mockWorkflowRun("waiting", "")},
				".github/workflows/test8.yml": {mockWorkflowRun("requested", "")},
				".github/workflows/test9.yml": {mockWorkflowRun("startup_failure", "")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"any"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values: []string{
					".github/workflows/test.yml",
					".github/workflows/test2.yml",
					".github/workflows/test3.yml",
					".github/workflows/test4.yml",
					".github/workflows/test5.yml",
					".github/workflows/test6.yml",
					".github/workflows/test7.yml",
					".github/workflows/test8.yml",
					".github/workflows/test9.yml",
				},
			},
		},
		{
			name: "a any state is allowed, it should match only successful conclusions by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "skipped")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"any"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".github/workflows/test2.yml"},
			},
		},
		{
			name: "a any state is allowed and all workflows succeded, it should statisfy by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":  {mockWorkflowRun("completed", "success")},
				".github/workflows/test2.yml": {mockWorkflowRun("completed", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses:  []string{"any"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/test.yml", ".github/workflows/test2.yml"},
			},
		},
		{
			name: "a predicate with regex syntax is treated as regex by default",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":       {mockWorkflowRun("completed", "success")},
				".github/workflows/abctestabc.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test3.yml":      {mockWorkflowRun("completed", "success")},
				".github/workflows/abc.yml":        {mockWorkflowRun("in_progress", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*test.*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{".github/workflows/abctestabc.yml", ".github/workflows/test.yml", ".github/workflows/test3.yml"},
			},
		},
		{
			name: "a predicate with regex syntax is treated as literal when noRegex is set",
			latestWorkflowRunsValue: map[string][]*github.WorkflowRun{
				".github/workflows/test.yml":       {mockWorkflowRun("completed", "success")},
				".github/workflows/abctestabc.yml": {mockWorkflowRun("completed", "success")},
				".github/workflows/test3.yml":      {mockWorkflowRun("completed", "success")},
				".github/workflows/abc.yml":        {mockWorkflowRun("in_progress", "success")},
			},
			predicate: HasWorkflow{
				Workflows: []common.Regexp{common.NewMustCompileRegexp(".*test.*")},
				noRegex:   true,
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".*test.*"},
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
				LatestWorkflowRunsError: tc.latestWorkflowRunsError,
			})
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
