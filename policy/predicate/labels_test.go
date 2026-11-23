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
	"errors"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestLabels(t *testing.T) {
	runLabelsPredicateTestCases(t, []LabelsTestCase{
		{
			"has_any matches",
			Labels{HasAny: []string{"enhancement", "bug"}},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied:     true,
				Values:        []string{"enhancement"},
				ConditionsMap: map[string][]string{"has any of": {"enhancement", "bug"}},
			},
		},
		{
			"has_any does not match",
			Labels{HasAny: []string{"enhancement", "bug"}},
			&pulltest.Context{LabelsValue: []string{"documentation"}},
			&common.PredicateResult{
				Satisfied:     false,
				Values:        []string{"documentation"},
				ConditionsMap: map[string][]string{"has any of": {"enhancement", "bug"}},
			},
		},
		{
			"has_all with all present",
			Labels{HasAll: []string{"enhancement", "bug"}},
			&pulltest.Context{LabelsValue: []string{"bug", "enhancement", "wontfix"}},
			&common.PredicateResult{
				Satisfied:     true,
				Values:        []string{"bug", "enhancement", "wontfix"},
				ConditionsMap: map[string][]string{"has all of": {"enhancement", "bug"}},
			},
		},
		{
			"has_all with one missing",
			Labels{HasAll: []string{"enhancement", "bug"}},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied:     false,
				Values:        []string{"enhancement"},
				ConditionsMap: map[string][]string{"has all of": {"enhancement", "bug"}},
			},
		},
		{
			"has_none with none present",
			Labels{HasNone: []string{"do-not-merge"}},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied:     true,
				Values:        []string{"enhancement"},
				ConditionsMap: map[string][]string{"has none of": {"do-not-merge"}},
			},
		},
		{
			"has_none with one present",
			Labels{HasNone: []string{"do-not-merge", "blocked"}},
			&pulltest.Context{LabelsValue: []string{"blocked"}},
			&common.PredicateResult{
				Satisfied:     false,
				Values:        []string{"blocked"},
				ConditionsMap: map[string][]string{"has none of": {"do-not-merge", "blocked"}},
			},
		},
		{
			"combined subkeys all satisfied",
			Labels{HasAny: []string{"enhancement"}, HasNone: []string{"do-not-merge"}},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"enhancement"},
				ConditionsMap: map[string][]string{
					"has any of":  {"enhancement"},
					"has none of": {"do-not-merge"},
				},
			},
		},
		{
			"combined subkeys one unsatisfied",
			Labels{HasAny: []string{"enhancement"}, HasNone: []string{"do-not-merge"}},
			&pulltest.Context{LabelsValue: []string{"enhancement", "do-not-merge"}},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"enhancement", "do-not-merge"},
				ConditionsMap: map[string][]string{
					"has any of":  {"enhancement"},
					"has none of": {"do-not-merge"},
				},
			},
		},
		{
			"matching is case-insensitive",
			Labels{HasAll: []string{"Enhancement"}, HasNone: []string{"Do-Not-Merge"}},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"enhancement"},
				ConditionsMap: map[string][]string{
					"has all of":  {"Enhancement"},
					"has none of": {"Do-Not-Merge"},
				},
			},
		},
		{
			"no labels present",
			Labels{HasAll: []string{"enhancement"}, HasNone: []string{"do-not-merge"}},
			&pulltest.Context{LabelsValue: nil},
			&common.PredicateResult{
				Satisfied: false,
				Values:    nil,
				ConditionsMap: map[string][]string{
					"has all of":  {"enhancement"},
					"has none of": {"do-not-merge"},
				},
			},
		},
		{
			"no subkeys set is trivially satisfied",
			Labels{},
			&pulltest.Context{LabelsValue: []string{"enhancement"}},
			&common.PredicateResult{
				Satisfied:     true,
				Values:        []string{"enhancement"},
				ConditionsMap: map[string][]string{},
			},
		},
	})
}

func TestLabelsError(t *testing.T) {
	p := Labels{HasAny: []string{"enhancement"}}
	_, err := p.Evaluate(context.Background(), &pulltest.Context{
		LabelsError: errors.New("failed to list labels"),
	})
	assert.Error(t, err, "expected an error when listing labels fails")
}

type LabelsTestCase struct {
	name                    string
	predicate               Labels
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runLabelsPredicateTestCases(t *testing.T, cases []LabelsTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := tc.predicate.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
