// Copyright 2025 Palantir Technologies, Inc.
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
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CustomPropertyIsNull []string
type CustomPropertyIsNotNull []string
type CustomPropertyMatchesAnyOf map[string][]common.Regexp
type CustomPropertyMatchesNoneOf map[string][]common.Regexp

var _ Predicate = (CustomPropertyIsNull)(nil)
var _ Predicate = (CustomPropertyIsNotNull)(nil)
var _ Predicate = (CustomPropertyMatchesAnyOf)(nil)
var _ Predicate = (CustomPropertyMatchesNoneOf)(nil)

func formatCustomProperties(customProperties map[string]pull.CustomProperty) []string {
	result := []string{}
	keys := slices.Sorted(maps.Keys(customProperties))
	for _, k := range keys {
		v := customProperties[k]
		formatted := "(null)"
		if v.String != nil {
			formatted = *v.String
		}
		if v.Array != nil {
			formatted = "[" + strings.Join(v.Array, ", ") + "]"
		}
		result = append(result, fmt.Sprintf("%s: %s", k, formatted))
	}
	return result
}

func (pred CustomPropertyIsNotNull) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	customProperties, err := prctx.RepositoryCustomProperties()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository custom properties")
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "custom properties",
		Values:          formatCustomProperties(customProperties),
		ConditionPhrase: "contain",
		ConditionValues: pred,
		Satisfied:       true,
	}

	for _, property := range pred {
		// For custom properties, empty strings and empty arrays are considered unset, and are not returned from the API.
		if _, ok := customProperties[property]; !ok {
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	return &predicateResult, nil
}

func (pred CustomPropertyIsNull) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	customProperties, err := prctx.RepositoryCustomProperties()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository custom properties")
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:       "custom properties",
		Values:            formatCustomProperties(customProperties),
		ReverseSkipPhrase: true,
		ConditionPhrase:   "contain",
		ConditionValues:   pred,
		Satisfied:         true,
	}

	for _, property := range pred {
		// For custom properties, empty strings and empty arrays are considered unset, and are not returned from the API.
		if _, ok := customProperties[property]; ok {
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	return &predicateResult, nil
}

func (pred CustomPropertyMatchesAnyOf) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	customProperties, err := prctx.RepositoryCustomProperties()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository custom properties")
	}

	conditionsMap := make(map[string][]string, len(pred))
	for property, allowedValues := range pred {
		strValues := make([]string, len(allowedValues))
		for i, v := range allowedValues {
			strValues[i] = v.String()
		}
		conditionsMap[property] = strValues
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "custom properties",
		Values:          formatCustomProperties(customProperties),
		ConditionPhrase: "match one or more of the patterns",
		ConditionsMap:   conditionsMap,
		Satisfied:       true,
	}

	for property, allowedValues := range pred {
		propValue, ok := customProperties[property]
		if !ok {
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}

		if propValue.String == nil || !anyMatches(allowedValues, *propValue.String) {
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	return &predicateResult, nil
}

func (pred CustomPropertyMatchesNoneOf) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	customProperties, err := prctx.RepositoryCustomProperties()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository custom properties")
	}

	conditionsMap := make(map[string][]string, len(pred))
	for property, allowedValues := range pred {
		strValues := make([]string, len(allowedValues))
		for i, v := range allowedValues {
			strValues[i] = v.String()
		}
		conditionsMap[property] = strValues
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:       "custom properties",
		Values:            formatCustomProperties(customProperties),
		ReverseSkipPhrase: true,
		ConditionPhrase:   "match one or more of the patterns",
		ConditionsMap:     conditionsMap,
		Satisfied:         true,
	}

	for property, disallowedValues := range pred {
		propValue, ok := customProperties[property]
		if !ok {
			continue
		}

		if propValue.String != nil && anyMatches(disallowedValues, *propValue.String) {
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	return &predicateResult, nil
}

func (pred CustomPropertyIsNotNull) Trigger() common.Trigger     { return common.TriggerStatic }
func (pred CustomPropertyIsNull) Trigger() common.Trigger        { return common.TriggerStatic }
func (pred CustomPropertyMatchesAnyOf) Trigger() common.Trigger  { return common.TriggerStatic }
func (pred CustomPropertyMatchesNoneOf) Trigger() common.Trigger { return common.TriggerStatic }
