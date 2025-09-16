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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T {
	return &v
}

var customPropertiesTestCtx = &pulltest.Context{
	RepositoryCustomPropertiesValue: map[string]pull.CustomProperty{
		"custom1": {String: ptr("value1")},
		"custom2": {String: ptr("value2")},
		"custom3": {Array: []string{"a", "b", "c"}},
	},
}

var defaultTestCustomPropertiesValues = []string{
	"custom1: value1",
	"custom2: value2",
	"custom3: [a, b, c]",
}

func TestCustomPropertiesIsNotNull(t *testing.T) {
	runCustomPropertyTestCase(t, customPropertiesTestCtx, []customPropertyTestCase{
		{
			description: "property is present and not null",
			predicate:   CustomPropertyIsNotNull{"custom1", "custom2", "custom3"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       true,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2", "custom3"},
			},
		},
		{
			description: "some properties only are present",
			predicate:   CustomPropertyIsNotNull{"custom1", "custom2", "custom3", "custom4"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       false,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2", "custom3", "custom4"},
			},
		},
		{
			description: "some properties only are checked",
			predicate:   CustomPropertyIsNotNull{"custom1", "custom2"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       true,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2"},
			},
		},
		{
			description: "no properties are present",
			predicate:   CustomPropertyIsNotNull{"custom4", "custom5"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       false,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom4", "custom5"},
			},
		},
		{
			description: "no properties are checked",
			predicate:   CustomPropertyIsNotNull{},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       true,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{},
			},
		},
	})
}

func TestCustomPropertiesIsNull(t *testing.T) {
	runCustomPropertyTestCase(t, customPropertiesTestCtx, []customPropertyTestCase{
		{
			description: "property is present and not null",
			predicate:   CustomPropertyIsNull{"custom1", "custom2", "custom3"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       false,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2", "custom3"},
			},
		},
		{
			description: "some properties only are present",
			predicate:   CustomPropertyIsNull{"custom1", "custom2", "custom3", "custom4"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       false,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2", "custom3", "custom4"},
			},
		},
		{
			description: "some properties only are checked",
			predicate:   CustomPropertyIsNull{"custom1", "custom2"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       false,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom1", "custom2"},
			},
		},
		{
			description: "no properties are present",
			predicate:   CustomPropertyIsNull{"custom4", "custom5"},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       true,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{"custom4", "custom5"},
			},
		},
		{
			description: "no properties are checked",
			predicate:   CustomPropertyIsNull{},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:       true,
				Values:          defaultTestCustomPropertiesValues,
				ConditionValues: []string{},
			},
		},
	})
}

func TestCustomPropertiesMatchesAnyOf(t *testing.T) {
	runCustomPropertyTestCase(t, customPropertiesTestCtx, []customPropertyTestCase{
		{
			description: "property matches regex",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`.*`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`.*`}},
			},
		},
		{
			description: "property does not match regex",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`nomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}},
			},
		},
		{
			description: "multiple regexes and one matches",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`nomatch`, `value1`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`, `value1`}},
			},
		},
		{
			description: "multiple regexes and none match",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`nomatch`, `alsonomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`, `alsonomatch`}},
			},
		},
		{
			description: "multiple properties and one matches",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`nomatch`}, "custom2": []string{`value2`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}, "custom2": {`value2`}},
			},
		},
		{
			description: "multiple properties and none match",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`nomatch`}, "custom2": []string{`alsonomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}, "custom2": {`alsonomatch`}},
			},
		},
		{
			description: "array should never match",
			predicate:   CustomPropertyMatchesAnyOf{"custom3": []string{`.*`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom3": {`.*`}},
			},
		},
		{
			description: "invalid regex should err",
			predicate:   CustomPropertyMatchesAnyOf{"custom1": []string{`*`}},
			ExpectedErr: func(err error) bool {
				return err.Error() == "failed to compile regex * for custom property custom1: error parsing regexp: missing argument to repetition operator: `*`"
			},
		},
		{
			description: "no properties are checked",
			predicate:   CustomPropertyMatchesAnyOf{},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{},
			},
		},
	})
}

func TestCustomPropertiesMatchesNoneOf(t *testing.T) {
	runCustomPropertyTestCase(t, customPropertiesTestCtx, []customPropertyTestCase{
		{
			description: "property matches regex",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`.*`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`.*`}},
			},
		},
		{
			description: "property does not match regex",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`nomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}},
			},
		},
		{
			description: "multiple regexes and one matches",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`nomatch`, `value1`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`, `value1`}},
			},
		},
		{
			description: "multiple regexes and none match",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`nomatch`, `alsonomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`, `alsonomatch`}},
			},
		},
		{
			description: "multiple properties and one matches",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`nomatch`}, "custom2": []string{`value2`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     false,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}, "custom2": {`value2`}},
			},
		},
		{
			description: "multiple properties and none match",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`nomatch`}, "custom2": []string{`alsonomatch`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom1": {`nomatch`}, "custom2": {`alsonomatch`}},
			},
		},
		{
			description: "array should never match",
			predicate:   CustomPropertyMatchesNoneOf{"custom3": []string{`.*`}},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{"custom3": {`.*`}},
			},
		},
		{
			description: "invalid regex should err",
			predicate:   CustomPropertyMatchesNoneOf{"custom1": []string{`*`}},
			ExpectedErr: func(err error) bool {
				return err.Error() == "failed to compile regex * for custom property custom1: error parsing regexp: missing argument to repetition operator: `*`"
			},
		},
		{
			description: "no properties are checked",
			predicate:   CustomPropertyMatchesNoneOf{},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied:     true,
				Values:        defaultTestCustomPropertiesValues,
				ConditionsMap: map[string][]string{},
			},
		},
	})
}

type customPropertyTestCase struct {
	description             string
	predicate               Predicate
	ExpectedPredicateResult *common.PredicateResult
	ExpectedErr             func(error) bool
}

func runCustomPropertyTestCase(t *testing.T, prctx pull.Context, cases []customPropertyTestCase) {
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			predicateResult, err := tc.predicate.Evaluate(ctx, prctx)
			if tc.ExpectedErr != nil {
				if !assert.Error(t, err, "expected error but got none") {
					return
				}
				if !tc.ExpectedErr(err) {
					t.Errorf("error did not match expectation: %v", err)
				}
			} else if assert.NoError(t, err, "predicate evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
