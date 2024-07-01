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
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasStatus struct {
	Conclusions allowedConclusions `yaml:"conclusions"`
	Statuses    []string           `yaml:"statuses"`
}

func NewHasStatus(statuses []string, conclusions []string) *HasStatus {
	conclusionsSet := make(allowedConclusions, len(conclusions))
	for _, conclusion := range conclusions {
		conclusionsSet[conclusion] = unit{}
	}
	return &HasStatus{
		Conclusions: conclusionsSet,
		Statuses:    statuses,
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for HasStatus.
// This supports unmarshalling the predicate in two forms:
//  1. A list of strings, which are the statuses to check for. This is the
//     deprecated `has_successful_status` format.
//  2. A full structure with `statuses` and `conclusions` fields as in
//     `has_status`.
func (pred *HasStatus) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a list of strings first
	statuses := []string{}
	if err := unmarshal(&statuses); err == nil {
		pred.Statuses = statuses

		return nil
	}

	// If that fails, try to unmarshal as the full structure
	type rawHasSuccessfulStatus HasStatus
	return unmarshal((*rawHasSuccessfulStatus)(pred))
}

var _ Predicate = HasStatus{}

func (pred HasStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	conclusions := pred.Conclusions
	if len(conclusions) == 0 {
		conclusions = defaultConclusions
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
		if _, allowed := conclusions[result]; !allowed {
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
