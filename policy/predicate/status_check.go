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
	"regexp"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasStatusCheck struct {
	Conclusions AllowedConclusions `yaml:"conclusions"`
	Statuses    AllowedStatuses    `yaml:"statuses"`
	Checks      []common.Regexp    `yaml:"checks,omitempty"`
	noRegex     bool
}

func NewHasStatusCheck(checks []common.Regexp, statuses []string, conclusions []string) *HasStatusCheck {
	return &HasStatusCheck{
		Conclusions: conclusions,
		Statuses:    statuses,
		Checks:      checks,
		noRegex:     false,
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
	var allChecks []string
	for _, check := range pred.Checks {
		matched := false
		check_to_use := check
		if pred.noRegex {
			check_to_use, err = common.NewRegexp(fmt.Sprintf("^%s$", regexp.QuoteMeta(check.String())))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create regexp for workflow %s", check.String())
			}
		}
		for checkResultName, checkResult := range checkStatuses {
			if check_to_use.Matches(checkResultName) {
				matched = true
				allChecks = append(allChecks, checkResultName)
				isInvalidConclusion := !slices.Contains(allowedConclusions, *checkResult.Conclusion)
				if (checkResult.Status == nil || !slices.Contains(allowedStatuses, *checkResult.Status)) ||
					(slices.Contains(allowedStatuses, "completed") && checkResult.Conclusion != nil && isInvalidConclusion) {
					failingStatuses = append(failingStatuses, checkResultName)
				}
			}
		}
		for repoStatusName, repoStatusResult := range repoStatuses {
			if check_to_use.Matches(repoStatusName) {
				matched = true
				allChecks = append(allChecks, repoStatusName)
				if repoStatusResult == nil || repoStatusResult.State == nil || !slices.Contains(allowedStatuses, *repoStatusResult.State) {
					failingStatuses = append(failingStatuses, repoStatusName)
				}
			}
		}
		if !matched {
			missingResults = append(missingResults, check.String())
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

	predicateResult.Values = allChecks
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasStatusCheck) Trigger() common.Trigger {
	return common.TriggerStatus
}
