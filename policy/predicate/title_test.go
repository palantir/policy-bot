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
			true,
			&pulltest.Context{
				TitleValue: "",
			},
		},
		{
			"matches pattern",
			false,
			&pulltest.Context{
				TitleValue: "chore: added tests",
			},
		},
		{
			"does not match pattern",
			true,
			&pulltest.Context{
				TitleValue: "changes: added tests",
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
			false,
			&pulltest.Context{
				TitleValue: "",
			},
		},
		{
			"matches pattern",
			true,
			&pulltest.Context{
				TitleValue: "BLOCKED: new feature",
			},
		},
		{
			"does not match pattern",
			false,
			&pulltest.Context{
				TitleValue: "feat: new feature",
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
			true,
			&pulltest.Context{
				TitleValue: "",
			},
		},
		{
			"matches first pattern in matches list",
			false,
			&pulltest.Context{
				TitleValue: "fix: fixes failing tests",
			},
		},
		{
			"matches second pattern in matches list",
			false,
			&pulltest.Context{
				TitleValue: "BREAKING CHANGE: new api version",
			},
		},
		{
			"matches pattern in not_matches list",
			true,
			&pulltest.Context{
				TitleValue: "BLOCKED: not working",
			},
		},
		{
			"matches pattern in both lists",
			true,
			&pulltest.Context{
				TitleValue: "BREAKING CHANGE: BLOCKED",
			},
		},
		{
			"does not match any pattern",
			true,
			&pulltest.Context{
				TitleValue: "test: adds tests",
			},
		},
	})
}

type TitleTestCase struct {
	name     string
	expected bool
	context  pull.Context
}

func runTitleTestCase(t *testing.T, p Predicate, cases []TitleTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, _, _, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.expected, ok, "predicate was not correct")
			}
		})
	}
}
