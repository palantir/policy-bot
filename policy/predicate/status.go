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
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type hasSuccessfulStatusOptions struct {
	SkippedIsSuccess bool `yaml:"skipped_is_success"`
}

type HasSuccessfulStatus struct {
	Options hasSuccessfulStatusOptions
	Statuses []string `yaml:"statuses"`
}

func NewHasSuccessfulStatus(statuses []string) *HasSuccessfulStatus {
	return &HasSuccessfulStatus{
		Statuses: statuses,
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for HasSuccessfulStatus.
// This allows the predicate to be specified as either a list of strings or with options.
func (pred *HasSuccessfulStatus) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a list of strings first
	statuses := []string{}
	if err := unmarshal(&statuses); err == nil {
		pred.Statuses = statuses

		return nil
	}

	// If that fails, try to unmarshal as the full structure
	type rawHasSuccessfulStatus HasSuccessfulStatus
	return unmarshal((*rawHasSuccessfulStatus)(pred))
}

var _ Predicate = HasSuccessfulStatus{}

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	allowedStatusConclusions := map[string]struct{}{
		"success": {},
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "status checks",
		ConditionPhrase: "exist and pass",
	}

	if pred.Options.SkippedIsSuccess {
		predicateResult.ConditionPhrase += " or are skipped"
		allowedStatusConclusions["skipped"] = struct{}{}
	}

	var missingResults []string
	var failingStatuses []string
	for _, status := range pred.Statuses {
		result, ok := statuses[status]
		if !ok {
			missingResults = append(missingResults, status)
		}
		if _, allowed := allowedStatusConclusions[result]; !allowed {
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
		predicateResult.Description = "One or more statuses has not passed: " + strings.Join(failingStatuses, ",")
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	predicateResult.Values = pred.Statuses
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}
