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
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestBranches(t *testing.T) {
	runBranchesTestCase(t, "^master$", []branchesTestCase{
		{
			"simple match - master",
			"master",
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"master"},
				ConditionValues: []string{"^master$"},
			},
		},
		{
			"simple non match",
			"another-branch",
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"another-branch"},
				ConditionValues: []string{"^master$"},
			},
		},
		{
			"tests anchoring",
			"not-master",
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"not-master"},
				ConditionValues: []string{"^master$"},
			},
		},
	})

	runBranchesTestCase(t, ".*", []branchesTestCase{
		{
			"matches all example 1",
			"master",
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"master"},
				ConditionValues: []string{".*"},
			},
		},
		{
			"matches all example 2",
			"another-one",
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"another-one"},
				ConditionValues: []string{".*"},
			},
		},
	})

	runBranchesTestCase(t, "(prod|staging)", []branchesTestCase{
		{
			"matches pattern - prod",
			"prod",
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"prod"},
				ConditionValues: []string{"(prod|staging)"},
			},
		},
		{
			"matches pattern - staging",
			"staging",
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"staging"},
				ConditionValues: []string{"(prod|staging)"},
			},
		},
		{
			"matches pattern - not-a-match",
			"not-a-match",
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"not-a-match"},
				ConditionValues: []string{"(prod|staging)"},
			},
		},
	})
}

// TODO: generalize this and use it all our test cases
type branchesTestCase struct {
	name                    string
	branchName              string
	ExpectedPredicateResult *common.PredicateResult
}

func runBranchesTestCase(t *testing.T, regex string, cases []branchesTestCase) {
	ctx := context.Background()

	for _, tc := range cases {

		compiledRegexp := common.NewCompiledRegexp(regexp.MustCompile(regex))
		targetsPredicate := &TargetsBranch{
			Pattern: compiledRegexp,
		}
		fromPredicate := &FromBranch{
			Pattern: compiledRegexp,
		}

		targetsContext := &pulltest.Context{
			BranchBaseName: tc.branchName,
		}
		fromContext := &pulltest.Context{
			BranchHeadName: tc.branchName,
		}

		t.Run(tc.name+" targets_branch", func(t *testing.T) {
			predicateResult, err := targetsPredicate.Evaluate(ctx, targetsContext)
			if assert.NoError(t, err, "targets_branch predicate evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})

		t.Run(tc.name+" from_branch", func(t *testing.T) {
			predicateResult, err := fromPredicate.Evaluate(ctx, fromContext)
			if assert.NoError(t, err, "from_branch predicate evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
