// Copyright 2022 Palantir Technologies, Inc.
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

func TestRepositoryWithNotMatchRule(t *testing.T) {
	p := &Repository{
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("palantir/docs")),
		},
		Matches: []common.Regexp{},
	}

	runRepositoryTestCase(t, p, []RepositoryTestCase{
		{
			"matches pattern",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "docs",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"palantir/docs"},
				ConditionsMap: map[string][]string{
					"not match": {"palantir/docs"},
					"match":     nil,
				},
			},
		},
		{
			"does not match pattern",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "policy-bot",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"palantir/policy-bot"},
				ConditionsMap: map[string][]string{
					"not match": {"palantir/docs"},
				},
			},
		},
	})
}

func TestRepositoryWithMatchRule(t *testing.T) {
	p := &Repository{
		NotMatches: []common.Regexp{},
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("palantir/policy.*")),
		},
	}

	runRepositoryTestCase(t, p, []RepositoryTestCase{
		{
			"matches pattern",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "policy-bot",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"palantir/policy-bot"},
				ConditionsMap: map[string][]string{
					"match": {"palantir/policy.*"},
				},
			},
		},
		{
			"does not match pattern",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "nanda-bot",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"palantir/nanda-bot"},
				ConditionsMap: map[string][]string{
					"not match": nil,
					"match":     {"palantir/policy.*"},
				},
			},
		},
	})
}

func TestRepositoryWithMixedRules(t *testing.T) {
	p := &Repository{
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("palantir/.*docs")),
			common.NewCompiledRegexp(regexp.MustCompile("palantir/special-repo")),
		},
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("palantir/policy.*")),
		},
	}

	runRepositoryTestCase(t, p, []RepositoryTestCase{
		{
			"matches pattern in match list",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "policy-bot",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"palantir/policy-bot"},
				ConditionsMap: map[string][]string{
					"match": {"palantir/policy.*"},
				},
			},
		},
		{
			"matches pattern in not_match list",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "docs",
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"palantir/docs"},
				ConditionsMap: map[string][]string{
					"not match": {"palantir/.*docs", "palantir/special-repo"},
					"match":     {"palantir/policy.*"},
				},
			},
		},
		{
			"matches pattern in both lists",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "policy-bot-docs",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"palantir/policy-bot-docs"},
				ConditionsMap: map[string][]string{
					"match": {"palantir/policy.*"},
				},
			},
		},
		{
			"does not match any pattern",
			&pulltest.Context{
				OwnerValue: "palantir",
				RepoValue:  "some-other-repo",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"palantir/some-other-repo"},
				ConditionsMap: map[string][]string{
					"not match": {"palantir/.*docs", "palantir/special-repo"},
				},
			},
		},
	})
}

type RepositoryTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runRepositoryTestCase(t *testing.T, p Predicate, cases []RepositoryTestCase) {
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
