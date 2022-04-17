// Copyright 2019 Palantir Technologies, Inc.
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

func TestHasLabels(t *testing.T) {
	p := HasLabels([]string{"foo", "bar"})

	runLabelsTestCase(t, p, []HasLabelsTestCase{
		{
			"all labels",
			&pulltest.Context{
				LabelsValue: []string{"foo", "bar"},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"foo", "bar"},
				ConditionValues: []string{"foo", "bar"},
			},
		},
		{
			"missing a label",
			&pulltest.Context{
				LabelsValue: []string{"foo"},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"foo"},
				ConditionValues: []string{"bar"},
			},
		},
		{
			"no labels",
			&pulltest.Context{
				LabelsValue: []string{},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{},
				ConditionValues: []string{"foo"},
			},
		},
	})
}

type HasLabelsTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runLabelsTestCase(t *testing.T, p Predicate, cases []HasLabelsTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.ExpectedPredicateResult.Satisfied, predicateResult.Satisfied, "predicate was not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.Values, predicateResult.Values, "values were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionsMap, predicateResult.ConditionsMap, "conditions were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionValues, predicateResult.ConditionValues, "conditions were not correct")
			}
		})
	}
}
