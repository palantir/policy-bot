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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CommitCount struct {
	Total ComparisonExpr `yaml:"total,omitempty"`
}

var _ Predicate = &CommitCount{}

func (pred *CommitCount) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	predicateResult := common.PredicateResult{
		ValuePhrase:     "commits",
		ConditionPhrase: "meet",
		ConditionsMap:   make(map[string][]string),
	}

	commits, err := prctx.Commits()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commits")
	}

	count := int64(len(commits))

	if !pred.Total.IsEmpty() {
		value := fmt.Sprintf("%d", count)
		cond := fmt.Sprintf("total commits %s", pred.Total.String())

		predicateResult.Values = []string{value}
		predicateResult.ConditionsMap["the commit count condition"] = []string{cond}

		if pred.Total.Evaluate(count) {
			predicateResult.Satisfied = true
			predicateResult.Description = fmt.Sprintf("PR has %d commits", count)
		} else {
			predicateResult.Satisfied = false
			predicateResult.Description = fmt.Sprintf("PR has %d commits, which does not meet the condition", count)
		}

		return &predicateResult, nil
	}

	// If no conditions are specified, default to unsatisfied
	predicateResult.Satisfied = false
	predicateResult.Description = "No commit count conditions specified"
	return &predicateResult, nil
}

func (pred *CommitCount) Trigger() common.Trigger {
	return common.TriggerCommit
}
