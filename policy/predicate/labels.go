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

// Labels is satisfied based on the set of labels applied to the pull request.
// Each sub-key is optional; when more than one is set, all must be satisfied
// (they combine with AND). Label matching is case-insensitive, matching the
// behavior of the has_labels predicate.
type Labels struct {
	HasAny  []string `yaml:"has_any,omitempty"`
	HasAll  []string `yaml:"has_all,omitempty"`
	HasNone []string `yaml:"has_none,omitempty"`
}

var _ Predicate = Labels{}

func (pred Labels) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	labels, err := prctx.Labels()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list pull request labels")
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "labels",
		Values:          labels,
		ConditionPhrase: "satisfy the label requirements",
		ConditionsMap:   map[string][]string{},
	}

	satisfied := true

	if len(pred.HasAny) > 0 {
		predicateResult.ConditionsMap["has any of"] = pred.HasAny
		if !anyLabel(labels, pred.HasAny) {
			satisfied = false
		}
	}
	if len(pred.HasAll) > 0 {
		predicateResult.ConditionsMap["has all of"] = pred.HasAll
		if !allLabels(labels, pred.HasAll) {
			satisfied = false
		}
	}
	if len(pred.HasNone) > 0 {
		predicateResult.ConditionsMap["has none of"] = pred.HasNone
		if anyLabel(labels, pred.HasNone) {
			satisfied = false
		}
	}

	predicateResult.Satisfied = satisfied
	predicateResult.Description = "The labels do not meet the required conditions"
	if satisfied {
		predicateResult.Description = "The labels meet the required conditions"
	}
	return &predicateResult, nil
}

func (pred Labels) Trigger() common.Trigger {
	return common.TriggerLabel
}

// anyLabel reports whether at least one of the required labels is present.
// prctx.Labels() returns lowercased values, so the configured labels are
// lowercased before comparison to keep matching case-insensitive.
func anyLabel(labels, required []string) bool {
	for _, r := range required {
		if contains(labels, strings.ToLower(r)) {
			return true
		}
	}
	return false
}

// allLabels reports whether every required label is present.
func allLabels(labels, required []string) bool {
	for _, r := range required {
		if !contains(labels, strings.ToLower(r)) {
			return false
		}
	}
	return true
}
