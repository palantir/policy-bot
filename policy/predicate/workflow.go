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
	"maps"
	"regexp"
	"slices"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasWorkflow struct {
	Statuses    []string        `yaml:"statuses,omitempty"`
	Conclusions []string        `yaml:"conclusions,omitempty"`
	Workflows   []common.Regexp `yaml:"workflows,omitempty"`
	noRegex     bool
}

func NewHasWorkflow(workflows []common.Regexp, conclusions []string, statuses []string) *HasWorkflow {
	return &HasWorkflow{
		Statuses:    statuses,
		Conclusions: conclusions,
		Workflows:   workflows,
		noRegex:     false,
	}
}

var _ Predicate = HasWorkflow{}

func (pred HasWorkflow) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	allowedConclusions := pred.Conclusions
	if len(allowedConclusions) == 0 {
		allowedConclusions = []string{"success"}
	} else if slices.Contains(allowedConclusions, "any") {
		allowedConclusions = []string{"action_required", "cancelled", "failure", "neutral", "skipped", "stale", "success", "timed_out"}
	}

	allowedStatuses := pred.Statuses
	if len(allowedStatuses) == 0 {
		allowedStatuses = []string{"completed"}
	} else if slices.Contains(allowedStatuses, "any") {
		allowedStatuses = []string{"completed", "expected", "failure", "in_progress", "pending", "queued", "requested", "startup_failure", "waiting"}
	}

	workflowRuns, err := prctx.LatestWorkflowRuns()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list latest workflow runs")
	}

	var missingResults []string
	var failingWorkflows []string
	var allWorkflows []string
	for _, workflow := range pred.Workflows {
		matched := false
		workflow_to_use := workflow
		if pred.noRegex {
			workflow_to_use, err = common.NewRegexp(fmt.Sprintf("^%s$", regexp.QuoteMeta(workflow.String())))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create regexp for workflow %s", workflow.String())
			}
		}
		for _, name := range slices.Sorted(maps.Keys(workflowRuns)) {
			if workflow_to_use.Matches(name) {
				matched = true
				allWorkflows = append(allWorkflows, name)
				for _, workflowResult := range workflowRuns[name] {
					isStatusAllowed := workflowResult.Status != nil && slices.Contains(allowedStatuses, *workflowResult.Status)
					isStatusCompletedAllowed := workflowResult.Status != nil && *workflowResult.Status == "completed" && slices.Contains(allowedStatuses, "completed")
					isConclusionAllowed := workflowResult.Conclusion != nil && slices.Contains(allowedConclusions, *workflowResult.Conclusion)
					if !isStatusAllowed || (isStatusCompletedAllowed && !isConclusionAllowed) {
						failingWorkflows = append(failingWorkflows, name)
					}
				}
			}
		}
		if !matched {
			missingResults = append(missingResults, workflow.String())
		}
	}

	predicateResult := common.PredicateResult{
		ValuePhrase: "workflow results",
		ConditionPhrase: fmt.Sprintf(
			"exist and have statuses %s and have conclusion(in case status is completed) %s: %s",
			joinElementsWithOr(allowedStatuses),
			joinElementsWithOr(allowedConclusions),
			allWorkflows,
		),
	}

	if len(missingResults) > 0 {
		predicateResult.Values = missingResults
		slices.Sort(missingResults)
		predicateResult.Description = fmt.Sprintf("One or more workflow runs are missing: %s", predicateResult.Values)
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	if len(failingWorkflows) > 0 {
		predicateResult.Values = failingWorkflows
		slices.Sort(failingWorkflows)
		predicateResult.Description = fmt.Sprintf(
			"One or more workflow runs have currently not status %s and/or conclusion(in case status is completed) %s: %s",
			joinElementsWithOr(allowedStatuses),
			joinElementsWithOr(allowedConclusions),
			failingWorkflows,
		)
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = allWorkflows
	slices.Sort(allWorkflows)
	predicateResult.Satisfied = true

	return &predicateResult, nil
}

func (pred HasWorkflow) Trigger() common.Trigger {
	return common.TriggerStatus
}
