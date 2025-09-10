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
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasStatusCheck struct {
	Conclusions []string        `yaml:"conclusions"`
	Statuses    []string        `yaml:"statuses"`
	Checks      []common.Regexp `yaml:"checks,omitempty"`
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
		allowedConclusions = []string{"success"}
	} else if slices.Contains(allowedConclusions, "any") {
		allowedConclusions = []string{"action_required", "cancelled", "failure", "neutral", "skipped", "stale", "success", "timed_out"}
	}

	allowedStatuses := pred.Statuses
	// success and error are statuses that only apply to the repo commit statuses.
	// pending and failure are statuses that apply to both repo commit statuses and check runs.
	// all other statuses are only applicable to check runs.
	if len(allowedStatuses) == 0 {
		allowedStatuses = []string{"completed", "success"}
	} else if slices.Contains(allowedStatuses, "any") {
		allowedStatuses = []string{"completed", "expected", "failure", "in_progress", "pending", "queued", "requested", "startup_failure", "waiting", "error", "success"}
	}

	checkStatuses, err := prctx.LatestCheckStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	repoStatuses, err := prctx.LatestRepoStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	var missingResults = make(map[string]string)
	var failingStatuses = make(map[string]string)
	var allChecks = make(map[string]string)
	for _, check := range pred.Checks {
		matched := false
		check_to_use := check
		if pred.noRegex {
			check_to_use, err = common.NewRegexp(fmt.Sprintf("^%s$", regexp.QuoteMeta(check.String())))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create regexp for workflow %s", check.String())
			}
		}
		for _, checkResultName := range slices.Sorted(maps.Keys(checkStatuses)) {
			if check_to_use.Matches(checkResultName) {
				matched = true
				allChecks[checkResultName] = checkResultName
				checkResult := checkStatuses[checkResultName]
				isValidStatus := slices.Contains(allowedStatuses, *checkResult.Status)
				isValidConclusion := checkResult.Conclusion != nil && slices.Contains(allowedConclusions, *checkResult.Conclusion)
				if (checkResult.Status == nil || !isValidStatus) ||
					(*checkResult.Status == "completed" && !isValidConclusion) {
					failingStatuses[checkResultName] = checkResultName
				}
			}
		}
		for _, repoStatusName := range slices.Sorted(maps.Keys(repoStatuses)) {
			if check_to_use.Matches(repoStatusName) {
				matched = true
				allChecks[repoStatusName] = repoStatusName
				repoStatusResult := repoStatuses[repoStatusName]
				if repoStatusResult == nil || repoStatusResult.State == nil || !slices.Contains(allowedStatuses, *repoStatusResult.State) {
					failingStatuses[repoStatusName] = repoStatusName
				}
			}
		}
		if !matched {
			missingResults[check.String()] = check.String()
		}
	}

	allChecksList := slices.Sorted(maps.Keys(allChecks))
	predicateResult := common.PredicateResult{
		ValuePhrase: "status checks and repo statuses",
		ConditionPhrase: fmt.Sprintf(
			"exist and have statuses %s and have conclusion(in case status is completed) %s: %s",
			joinElementsWithOr(allowedStatuses),
			joinElementsWithOr(allowedConclusions),
			allChecksList,
		),
	}

	if len(missingResults) > 0 {
		predicateResult.Values = slices.Sorted(maps.Keys(missingResults))
		predicateResult.Description = fmt.Sprintf("One or more status checks or repo statuses are missing: %s", predicateResult.Values)
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	if len(failingStatuses) > 0 {
		predicateResult.Values = slices.Sorted(maps.Keys(failingStatuses))
		predicateResult.Description = fmt.Sprintf("One or more status checks or repo statuses have not concluded with %s: %s", joinElementsWithOr(allowedConclusions), failingStatuses)
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = allChecksList
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasStatusCheck) Trigger() common.Trigger {
	return common.TriggerStatus
}

// joinWithOr returns a string that represents the allowed conclusions in a
// format that can be used in a sentence. For example, if the allowed
// conclusions/statuses are "success" and "failure", this will return "success or
// failure". If there are more than two conclusions/statuses, the first n-1 will be
// separated by commas.
func joinElementsWithOr(c []string) string {
	slices.Sort(c)

	length := len(c)
	switch length {
	case 0:
		return ""
	case 1:
		return c[0]
	case 2:
		return c[0] + " or " + c[1]
	}

	head, tail := c[:length-1], c[length-1]

	return strings.Join(head, ", ") + ", or " + tail
}
