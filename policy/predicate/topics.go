// Copyright 2026 Palantir Technologies, Inc.
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

type HasTopics []string

var _ Predicate = HasTopics([]string{})

func (pred HasTopics) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {

	predicateResult := common.PredicateResult{
		ValuePhrase:     "topics",
		ConditionPhrase: "contain the topics",
	}
	if len(pred) > 0 {
		topics, err := prctx.RepositoryTopics()
		if err != nil {
			return nil, errors.Wrap(err, "failed to list repository topics")
		}
		predicateResult.Values = topics
		for _, requiredTopic := range pred {
			if !contains(topics, strings.ToLower(requiredTopic)) {
				predicateResult.ConditionValues = []string{requiredTopic}
				predicateResult.Description = "Missing topic: " + requiredTopic
				predicateResult.Satisfied = false
				return &predicateResult, nil
			}
		}
	}
	predicateResult.ConditionValues = pred
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred HasTopics) Trigger() common.Trigger {
	return common.TriggerStatic
}
