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
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestChangedFiles(t *testing.T) {
	p := &ChangedFiles{
		Paths: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("app/.*\\.go")),
			common.NewCompiledRegexp(regexp.MustCompile("server/.*\\.go")),
		},
		IgnorePaths: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile(".*/special\\.go")),
		},
	}

	runFileTests(t, p, []FileTestCase{
		{
			"empty",
			[]*pull.File{},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
		{
			"onlyMatches",
			[]*pull.File{
				{
					Filename: "app/client.go",
					Status:   pull.FileAdded,
				},
				{
					Filename: "server/server.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"app/client.go"},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
		{
			"someMatches",
			[]*pull.File{
				{
					Filename: "app/client.go",
					Status:   pull.FileAdded,
				},
				{
					Filename: "model/user.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"app/client.go"},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
		{
			"noMatches",
			[]*pull.File{
				{
					Filename: "model/order.go",
					Status:   pull.FileDeleted,
				},
				{
					Filename: "model/user.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"model/order.go", "model/user.go"},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
		{
			"ignoreAll",
			[]*pull.File{
				{
					Filename: "app/special.go",
					Status:   pull.FileDeleted,
				},
				{
					Filename: "server/special.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"app/special.go", "server/special.go"},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
		{
			"ignoreSome",
			[]*pull.File{
				{
					Filename: "app/normal.go",
					Status:   pull.FileDeleted,
				},
				{
					Filename: "server/special.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"app/normal.go"},
				ConditionsMap: map[string][]string{
					"path patterns":        {"app/.*\\.go", "server/.*\\.go"},
					"while ignoring": {".*/special\\.go"},
				},
			},
		},
	})
}

func TestOnlyChangedFiles(t *testing.T) {
	p := &OnlyChangedFiles{
		Paths: []common.Regexp{
			common.NewCompiledRegexp(regexp.MustCompile("app/.*\\.go")),
			common.NewCompiledRegexp(regexp.MustCompile("server/.*\\.go")),
		},
	}

	runFileTests(t, p, []FileTestCase{
		{
			"empty",
			[]*pull.File{},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{},
				ConditionValues: []string{"app/.*\\.go", "server/.*\\.go"},
			},
		},
		{
			"onlyMatches",
			[]*pull.File{
				{
					Filename: "app/client.go",
					Status:   pull.FileAdded,
				},
				{
					Filename: "server/server.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"app/client.go", "server/server.go"},
				ConditionValues: []string{"app/.*\\.go", "server/.*\\.go"},
			},
		},
		{
			"someMatches",
			[]*pull.File{
				{
					Filename: "app/client.go",
					Status:   pull.FileAdded,
				},
				{
					Filename: "model/user.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"model/user.go"},
				ConditionValues: []string{"app/.*\\.go", "server/.*\\.go"},
			},
		},
		{
			"noMatches",
			[]*pull.File{
				{
					Filename: "model/order.go",
					Status:   pull.FileDeleted,
				},
				{
					Filename: "model/user.go",
					Status:   pull.FileModified,
				},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"model/order.go"},
				ConditionValues: []string{"app/.*\\.go", "server/.*\\.go"},
			},
		},
	})
}

func TestModifiedLines(t *testing.T) {
	p := &ModifiedLines{
		Additions: ComparisonExpr{Op: OpGreaterThan, Value: 100},
		Deletions: ComparisonExpr{Op: OpGreaterThan, Value: 10},
	}

	runFileTests(t, p, []FileTestCase{
		{
			"empty",
			[]*pull.File{},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"+0", "-0"},
				ConditionValues: []string{"added lines> 100", "deleted lines> 10"},
			},
		},
		{
			"additions",
			[]*pull.File{
				{Additions: 55},
				{Additions: 10},
				{Additions: 45},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"+110"},
				ConditionValues: []string{"added lines> 100"},
			},
		},
		{
			"deletions",
			[]*pull.File{
				{Additions: 5},
				{Additions: 10, Deletions: 10},
				{Additions: 5},
				{Deletions: 10},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"-20"},
				ConditionValues: []string{"deleted lines> 10"},
			},
		},
	})

	p = &ModifiedLines{
		Total: ComparisonExpr{Op: OpGreaterThan, Value: 100},
	}

	runFileTests(t, p, []FileTestCase{
		{
			"total",
			[]*pull.File{
				{Additions: 20, Deletions: 20},
				{Additions: 20},
				{Deletions: 20},
				{Additions: 20, Deletions: 20},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"total 120"},
				ConditionValues: []string{"total modifications> 100"},
			},
		},
	})
}

func TestComparisonExpr(t *testing.T) {
	tests := map[string]struct {
		Expr   ComparisonExpr
		Value  int64
		Output bool
	}{
		"greaterThanTrue": {
			Expr:   ComparisonExpr{Op: OpGreaterThan, Value: 100},
			Value:  200,
			Output: true,
		},
		"greaterThanFalse": {
			Expr:   ComparisonExpr{Op: OpGreaterThan, Value: 100},
			Value:  50,
			Output: false,
		},
		"lessThanTrue": {
			Expr:   ComparisonExpr{Op: OpLessThan, Value: 100},
			Value:  50,
			Output: true,
		},
		"lessThanFalse": {
			Expr:   ComparisonExpr{Op: OpLessThan, Value: 100},
			Value:  200,
			Output: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ok := test.Expr.Evaluate(test.Value)
			assert.Equal(t, test.Output, ok, "evaluation was not correct")
		})
	}

	t.Run("isEmpty", func(t *testing.T) {
		assert.True(t, ComparisonExpr{}.IsEmpty(), "expression was not empty")
		assert.False(t, ComparisonExpr{Op: OpGreaterThan, Value: 100}.IsEmpty(), "expression was empty")
	})

	parseTests := map[string]struct {
		Input  string
		Output ComparisonExpr
		Err    bool
	}{
		"lessThan": {
			Input:  "<100",
			Output: ComparisonExpr{Op: OpLessThan, Value: 100},
		},
		"greaterThan": {
			Input:  ">100",
			Output: ComparisonExpr{Op: OpGreaterThan, Value: 100},
		},
		"innerSpaces": {
			Input:  "<   35",
			Output: ComparisonExpr{Op: OpLessThan, Value: 35},
		},
		"leadingSpaces": {
			Input:  "   < 35",
			Output: ComparisonExpr{Op: OpLessThan, Value: 35},
		},
		"trailngSpaces": {
			Input:  "< 35   ",
			Output: ComparisonExpr{Op: OpLessThan, Value: 35},
		},
		"invalidOp": {
			Input: "=10",
			Err:   true,
		},
		"invalidValue": {
			Input: "< 10ab",
			Err:   true,
		},
	}

	for name, test := range parseTests {
		t.Run(name, func(t *testing.T) {
			var expr ComparisonExpr
			err := expr.UnmarshalText([]byte(test.Input))
			if test.Err {
				assert.Error(t, err, "expected error parsing expression, but got nil")
				return
			}
			if assert.NoError(t, err, "unexpected error parsing expression") {
				assert.Equal(t, test.Output, expr, "parsed expression was not correct")
			}
		})
	}
}

type FileTestCase struct {
	Name                    string
	Files                   []*pull.File
	ExpectedPredicateResult *common.PredicateResult
}

func runFileTests(t *testing.T, p Predicate, cases []FileTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			prctx := &pulltest.Context{
				ChangedFilesValue: tc.Files,
			}

			predicateResult, err := p.Evaluate(ctx, prctx)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.ExpectedPredicateResult.Satisfied, predicateResult.Satisfied, "predicate was not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.Values, predicateResult.Values, "values were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionsMap, predicateResult.ConditionsMap, "conditions were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionValues, predicateResult.ConditionValues, "conditions were not correct")
			}
		})
	}
}
