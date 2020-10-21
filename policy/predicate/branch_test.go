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

	"github.com/stretchr/testify/assert"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestTargetsBranch(t *testing.T) {
	p := &TargetsBranch{
		Pattern: common.NewCompiledRegexp(regexp.MustCompile("^master$")),
	}

	runTargetsTestCase(t, p, []targetsTestCase{
		{
			"simple match - master",
			true,
			&pulltest.Context{
				BranchBaseName: "master",
			},
		},
		{
			"simple non match",
			false,
			&pulltest.Context{
				BranchBaseName: "another-branch",
			},
		},
		{
			"tests anchoring",
			false,
			&pulltest.Context{
				BranchBaseName: "not-master",
			},
		},
	})

	pMatchAll := &TargetsBranch{
		Pattern: common.NewCompiledRegexp(regexp.MustCompile(".*")),
	}

	runTargetsTestCase(t, pMatchAll, []targetsTestCase{
		{
			"matches all example 1",
			true,
			&pulltest.Context{
				BranchBaseName: "master",
			},
		},
		{
			"matches all example 2",
			true,
			&pulltest.Context{
				BranchBaseName: "another-one",
			},
		},
	})

	pRegex := &TargetsBranch{
		Pattern: common.NewCompiledRegexp(regexp.MustCompile("(prod|staging)")),
	}

	runTargetsTestCase(t, pRegex, []targetsTestCase{
		{
			"matches pattern - prod",
			true,
			&pulltest.Context{
				BranchBaseName: "prod",
			},
		},
		{
			"matches pattern - staging",
			true,
			&pulltest.Context{
				BranchBaseName: "staging",
			},
		},
		{
			"matches pattern - not-a-match",
			false,
			&pulltest.Context{
				BranchBaseName: "not-a-match",
			},
		},
	})

	pSourceMaster := &SourceBranch{
		Pattern: common.NewCompiledRegexp(regexp.MustCompile("^master$")),
	}

	runTargetsTestCase(t, pSourceMaster, []targetsTestCase{
		{
			"simple match - master",
			true,
			&pulltest.Context{
				BranchHeadName: "master",
			},
		},
		{
			"simple non match",
			false,
			&pulltest.Context{
				BranchHeadName: "another-branch",
			},
		},
		{
			"tests anchoring",
			false,
			&pulltest.Context{
				BranchHeadName: "not-master",
			},
		},
	})
}

// TODO: generalize this and use it all our test cases
type targetsTestCase struct {
	name     string
	expected bool
	context  pull.Context
}

func runTargetsTestCase(t *testing.T, p Predicate, cases []targetsTestCase) {
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
