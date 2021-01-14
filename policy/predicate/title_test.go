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
	"github.com/palantir/policy-bot/policy/common"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestWithMatchRules(t *testing.T) {
	p := &Title{
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^[A-Z]*$")),
		},
		NotMatches: []common.Regexp{},
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
			true,
			&pulltest.Context{
				TitleValue: "UPDATE",
			},
		},
		{
			"does not match pattern",
			false,
			&pulltest.Context{
				TitleValue: "changes",
			},
		},
	})
}

func TestWithNotMatchRules(t *testing.T) {
	p := &Title{
		Matches: []common.Regexp{},
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^[a-z]*$")),
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
			false,
			&pulltest.Context{
				TitleValue: "update",
			},
		},
		{
			"does not match pattern",
			true,
			&pulltest.Context{
				TitleValue: "CHANGES",
			},
		},
	})
}

func TestWithMixedRules(t *testing.T) {
	p := &Title{
		Matches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^[A-Z]*$")),
		},
		NotMatches: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("^[a-z]*$")),
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
			"matches pattern in matches list",
			true,
			&pulltest.Context{
				TitleValue: "UPDATE",
			},
		},
		{
			"matches pattern in not_matches list",
			false,
			&pulltest.Context{
				TitleValue: "changes",
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
			ok, _, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.expected, ok, "predicate was not correct")
			}
		})
	}
}
