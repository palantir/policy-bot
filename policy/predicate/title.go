// Copyright 2021 Palantir Technologies, Inc.
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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type Title struct {
	Matches    []common.Regexp `yaml:"matches,omitempty"`
	NotMatches []common.Regexp `yaml:"not_matches,omitempty"`
}

var _ Predicate = Title{}

func (pred Title) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	title := prctx.Title()

	predicateResult := common.PredicateResult{
		ValuePhrase:     "titles",
		Values:          []string{title},
		ConditionPhrase: "meet the pattern requirement",
	}

	var matchPatterns, notMatchPatterns []string

	for _, reg := range pred.Matches {
		matchPatterns = append(matchPatterns, reg.String())
	}

	for _, reg := range pred.NotMatches {
		notMatchPatterns = append(notMatchPatterns, reg.String())
	}

	if len(pred.Matches) > 0 {
		if anyMatches(pred.Matches, title) {
			predicateResult.ConditionsMap = map[string][]string{"match": matchPatterns}
			predicateResult.Description = "PR Title matches a Match pattern"
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}

	if len(pred.NotMatches) > 0 {
		if !anyMatches(pred.NotMatches, title) {
			predicateResult.ConditionsMap = map[string][]string{"not match": notMatchPatterns}
			predicateResult.Description = "PR Title doesn't match a NotMatch pattern"
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}
	predicateResult.Satisfied = false
	predicateResult.ConditionsMap = map[string][]string{"match": matchPatterns, "not match": notMatchPatterns}
	return &predicateResult, nil
}

func (pred Title) Trigger() common.Trigger {
	return common.TriggerPullRequest
}
