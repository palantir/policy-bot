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
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestBranches(t *testing.T) {
	runBranchesTestCase(t, "^master$", []branchesTestCase{
		{
			"simple match - master",
			true,
			"master",
		},
		{
			"simple non match",
			false,
			"another-branch",
		},
		{
			"tests anchoring",
			false,
			"not-master",
		},
	})

	runBranchesTestCase(t, ".*", []branchesTestCase{
		{
			"matches all example 1",
			true,
			"master",
		},
		{
			"matches all example 2",
			true,
			"another-one",
		},
	})

	runBranchesTestCase(t, "(prod|staging)", []branchesTestCase{
		{
			"matches pattern - prod",
			true,
			"prod",
		},
		{
			"matches pattern - staging",
			true,
			"staging",
		},
		{
			"matches pattern - not-a-match",
			false,
			"not-a-match",
		},
	})
}

// TODO: generalize this and use it all our test cases
type branchesTestCase struct {
	name       string
	expected   bool
	branchName string
}

func runBranchesTestCase(t *testing.T, regex string, cases []branchesTestCase) {
	ctx := context.Background()

	for _, tc := range cases {

		compiledRegexp := common.NewCompiledRegexp(regexp.MustCompile(regex))
		targetsPredicate := &TargetsBranch{
			Pattern: compiledRegexp,
		}
		sourcePredicate := &SourceBranch{
			Pattern: compiledRegexp,
		}

		targetsContext := &pulltest.Context{
			BranchBaseName: tc.branchName,
		}
		sourceContext := &pulltest.Context{
			BranchHeadName: tc.branchName,
		}

		t.Run(tc.name+" targets", func(t *testing.T) {
			ok, _, err := targetsPredicate.Evaluate(ctx, targetsContext)
			if assert.NoError(t, err, "targets predicate evaluation failed") {
				assert.Equal(t, tc.expected, ok, "targets predicate was not correct")
			}
		})

		t.Run(tc.name+" source", func(t *testing.T) {
			ok, _, err := sourcePredicate.Evaluate(ctx, sourceContext)
			if assert.NoError(t, err, "source predicate evaluation failed") {
				assert.Equal(t, tc.expected, ok, "source predicate was not correct")
			}
		})
	}
}
