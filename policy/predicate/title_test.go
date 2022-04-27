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
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestWithNotMatchRule(t *testing.T) {
	p := &Title{
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^(fix|feat|chore): (\\w| )+$")),
		},
		Matches: []common.Regexp{},
	}

	runTitleTestCase(t, p, []TitleTestCase{
		{
			"empty title",
			&pulltest.Context{
				TitleValue: "",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{""},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$"},
				},
			},
		},
		{
			"matches pattern",
			&pulltest.Context{
				TitleValue: "chore: added tests",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"chore: added tests"},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$"},
					"match":     nil,
				},
			},
		},
		{
			"does not match pattern",
			&pulltest.Context{
				TitleValue: "changes: added tests",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"changes: added tests"},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$"},
				},
			},
		},
	})
}

func TestWithMatchRule(t *testing.T) {
	p := &Title{
		NotMatches: []common.Regexp{},
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^BLOCKED")),
		},
	}

	runTitleTestCase(t, p, []TitleTestCase{
		{
			"empty title",
			&pulltest.Context{
				TitleValue: "",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{""},
				ConditionsMap: map[string][]string{
					"not match": nil,
					"match":     {"^BLOCKED"},
				},
			},
		},
		{
			"matches pattern",
			&pulltest.Context{
				TitleValue: "BLOCKED: new feature",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"BLOCKED: new feature"},
				ConditionsMap: map[string][]string{
					"match": {"^BLOCKED"},
				},
			},
		},
		{
			"does not match pattern",
			&pulltest.Context{
				TitleValue: "feat: new feature",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"feat: new feature"},
				ConditionsMap: map[string][]string{
					"not match": nil,
					"match":     {"^BLOCKED"},
				},
			},
		},
	})
}

func TestWithMixedRules(t *testing.T) {
	p := &Title{
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^(fix|feat|chore): (\\w| )+$")),
			common.NewCompiledRegexp(regexp.MustCompile("^BREAKING CHANGE: (\\w| )+$")),
		},
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("BLOCKED")),
		},
	}

	runTitleTestCase(t, p, []TitleTestCase{
		{
			"empty title",
			&pulltest.Context{
				TitleValue: "",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{""},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$", "^BREAKING CHANGE: (\\w| )+$"},
				},
			},
		},
		{
			"matches first pattern in matches list",
			&pulltest.Context{
				TitleValue: "fix: fixes failing tests",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"fix: fixes failing tests"},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$", "^BREAKING CHANGE: (\\w| )+$"},
					"match":     {"BLOCKED"},
				},
			},
		},
		{
			"matches second pattern in matches list",
			&pulltest.Context{
				TitleValue: "BREAKING CHANGE: new api version",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"BREAKING CHANGE: new api version"},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$", "^BREAKING CHANGE: (\\w| )+$"},
					"match":     {"BLOCKED"},
				},
			},
		},
		{
			"matches pattern in not_matches list",
			&pulltest.Context{
				TitleValue: "BLOCKED: not working",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"BLOCKED: not working"},
				ConditionsMap: map[string][]string{
					"match": {"BLOCKED"},
				},
			},
		},
		{
			"matches pattern in both lists",
			&pulltest.Context{
				TitleValue: "BREAKING CHANGE: BLOCKED",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"BREAKING CHANGE: BLOCKED"},
				ConditionsMap: map[string][]string{
					"match": {"BLOCKED"},
				},
			},
		},
		{
			"does not match any pattern",
			&pulltest.Context{
				TitleValue: "test: adds tests",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"test: adds tests"},
				ConditionsMap: map[string][]string{
					"not match": {"^(fix|feat|chore): (\\w| )+$", "^BREAKING CHANGE: (\\w| )+$"},
				},
			},
		},
	})
}

type TitleTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runTitleTestCase(t *testing.T, p Predicate, cases []TitleTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
