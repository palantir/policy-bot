// Copyright 2018 Palantir Technologies, Inc.
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
	"fmt"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasWorkflowResult struct {
	Conclusions AllowedConclusions `yaml:"conclusions,omitempty"`
	Workflows   []string           `yaml:"workflows,omitempty"`
}

func NewHasWorkflowResult(workflows []string, conclusions []string) *HasWorkflowResult {
	return &HasWorkflowResult{
		Conclusions: conclusions,
		Workflows:   workflows,
	}
}

var _ Predicate = HasWorkflowResult{}

func (pred HasWorkflowResult) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	workflowRuns, err := prctx.LatestWorkflowRuns()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list latest workflow runs")
	}

	allowedConclusions := pred.Conclusions
	if len(allowedConclusions) == 0 {
		allowedConclusions = AllowedConclusions{"success"}
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "workflow results",
		ConditionPhrase: fmt.Sprintf("exist and have conclusion %s", allowedConclusions.joinWithOr()),
	}

	var missingResults []string
	var failingWorkflows []string
	for _, workflow := range pred.Workflows {
		conclusions, ok := workflowRuns[workflow]
		if !ok {
			missingResults = append(missingResults, workflow)
		}
		for _, conclusion := range conclusions {
			if !slices.Contains(allowedConclusions, conclusion) {
				failingWorkflows = append(failingWorkflows, workflow)
			}
		}
	}

	if len(missingResults) > 0 {
		predicateResult.Values = missingResults
		predicateResult.Description = "One or more workflow runs are missing: " + strings.Join(missingResults, ", ")
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	if len(failingWorkflows) > 0 {
		predicateResult.Values = failingWorkflows
		predicateResult.Description = fmt.Sprintf("One or more workflow runs have not concluded with %s: %s", pred.Conclusions.joinWithOr(), strings.Join(failingWorkflows, ","))
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = pred.Workflows
	predicateResult.Satisfied = true

	return &predicateResult, nil
}

func (pred HasWorkflowResult) Trigger() common.Trigger {
	return common.TriggerStatus
}
