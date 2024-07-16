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

type AllowedConclusions []string

type HasStatus struct {
	Conclusions AllowedConclusions `yaml:"conclusions"`
	Statuses    []string           `yaml:"statuses"`
}

func NewHasStatus(statuses []string, conclusions []string) *HasStatus {
	return &HasStatus{
		Conclusions: conclusions,
		Statuses:    statuses,
	}
}

var _ Predicate = HasStatus{}

func (pred HasStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	conclusions := pred.Conclusions
	if len(conclusions) == 0 {
		conclusions = AllowedConclusions{"success"}
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "status checks",
		ConditionPhrase: fmt.Sprintf("exist and have conclusion %s", conclusions.joinWithOr()),
	}

	var missingResults []string
	var failingStatuses []string
	for _, status := range pred.Statuses {
		result, ok := statuses[status]
		if !ok {
			missingResults = append(missingResults, status)
		}
		if !slices.Contains(conclusions, result) {
			failingStatuses = append(failingStatuses, status)
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
		predicateResult.Description = fmt.Sprintf("One or more statuses has not concluded with %s: %s", pred.Conclusions.joinWithOr(), strings.Join(failingStatuses, ","))
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = pred.Statuses
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// HasSuccessfulStatus checks that the specified statuses have a successful
// conclusion.
//
// Deprecated: use the more flexible `HasStatus` with `conclusions: ["success"]`
// instead.
type HasSuccessfulStatus []string

var _ Predicate = HasSuccessfulStatus{}

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	return HasStatus{
		Statuses: pred,
	}.Evaluate(ctx, prctx)
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// joinWithOr returns a string that represents the allowed conclusions in a
// format that can be used in a sentence. For example, if the allowed
// conclusions are "success" and "failure", this will return "success or
// failure". If there are more than two conclusions, the first n-1 will be
// separated by commas.
func (c AllowedConclusions) joinWithOr() string {
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
