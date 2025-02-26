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

type HasStatusCheck struct {
	Conclusions AllowedConclusions `yaml:"conclusions"`
	Statuses    AllowedStatuses    `yaml:"statuses"`
	Checks      []string           `yaml:"checks,omitempty"`
}

func NewHasStatusCheck(checks []string, statuses []string, conclusions []string) *HasStatusCheck {
	return &HasStatusCheck{
		Conclusions: conclusions,
		Statuses:    statuses,
		Checks:      checks,
	}
}

var _ Predicate = HasStatus{}

func (pred HasStatusCheck) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	allowedConclusions := pred.Conclusions
	if len(allowedConclusions) == 0 {
		allowedConclusions = AllowedConclusions{"success"}
	} else if slices.Contains(allowedConclusions, "any") {
		allowedConclusions = AllowedConclusions{"action_required", "cancelled", "failure", "neutral", "skipped", "stale", "success", "timed_out"}
	}

	allowedStatuses := pred.Statuses
	// success and error are statuses that only apply to the repo commit statuses.
	// pending and failure are statuses that apply to both repo commit statuses and check runs.
	// all other statuses are only applicable to check runs.
	if len(allowedStatuses) == 0 {
		allowedStatuses = AllowedStatuses{"completed", "success"}
	} else if slices.Contains(allowedStatuses, "any") {
		allowedStatuses = AllowedStatuses{"completed", "expected", "failure", "in_progress", "pending", "queued", "requested", "startup_failure", "waiting"} // "error", "success"
	}

	checkStatuses, err := prctx.LatestCheckStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	repoStatuses, err := prctx.LatestRepoStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "status checks",
		ConditionPhrase: fmt.Sprintf("exist and have conclusion %s", allowedConclusions.joinWithOr()),
	}

	var missingResults []string
	var failingStatuses []string
	for _, check := range pred.Checks {
		if checkResult, ok := checkStatuses[check]; ok {
			isInvalidConclusion := !slices.Contains(allowedConclusions, *checkResult.Conclusion)
			if (checkResult.Status == nil || !slices.Contains(allowedStatuses, *checkResult.Status)) ||
				(slices.Contains(allowedStatuses, "completed") && checkResult.Conclusion != nil && isInvalidConclusion) {
				failingStatuses = append(failingStatuses, check)
			}
		} else if repoStatusResult, ok := repoStatuses[check]; ok {
			if repoStatusResult == nil {
				missingResults = append(missingResults, check)
			} else if repoStatusResult.State == nil || !slices.Contains(allowedStatuses, *repoStatusResult.State) {
				failingStatuses = append(failingStatuses, check)
			}
		} else {
			missingResults = append(missingResults, check)
		}
	}

	if len(missingResults) > 0 {
		predicateResult.Values = missingResults
		predicateResult.Description = "One or more statuses is missing: " + strings.Join(missingResults, ", ")
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	if len(failingStatuses) > 0 {
		predicateResult.Values = failingStatuses
		predicateResult.Description = fmt.Sprintf("One or more statuses has not concluded with %s: %s", allowedConclusions.joinWithOr(), strings.Join(failingStatuses, ","))
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = pred.Checks
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasStatusCheck) Trigger() common.Trigger {
	return common.TriggerStatus
}
