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
			false,
			[]*pull.File{},
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			    },
			},
		},
		{
			"onlyMatches",
			true,
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
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			        ChangedFiles:   []string{"app/client.go"},
			    },
			},
		},
		{
			"someMatches",
			true,
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
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			        ChangedFiles:   []string{"app/client.go"},
			    },
			},
		},
		{
			"noMatches",
			false,
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
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			        ChangedFiles:   []string{"model/order.go", "model/user.go"},
			    },
			},
		},
		{
			"ignoreAll",
			false,
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
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			        ChangedFiles:   []string{"app/special.go", "server/special.go"},
			    },
			},
		},
		{
			"ignoreSome",
			true,
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
			&common.PredicateInfo{
			    Type: "ChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        IgnorePaths:    []string{".*/special\\.go"},
			        ChangedFiles:   []string{"app/normal.go"},
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
			false,
			[]*pull.File{},
			&common.PredicateInfo{
			    Type: "OnlyChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			    },
			},
		},
		{
			"onlyMatches",
			true,
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
			&common.PredicateInfo{
			    Type: "OnlyChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        ChangedFiles:   []string{"app/client.go", "server/server.go"},
			    },
			},
		},
		{
			"someMatches",
			false,
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
			&common.PredicateInfo{
			    Type: "OnlyChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        ChangedFiles:   []string{"model/user.go"},
			    },
			},
		},
		{
			"noMatches",
			false,
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
			&common.PredicateInfo{
			    Type: "OnlyChangedFiles",
			    Name: "Changed Files",
			    FileInfo: &common.FileInfo{
			        Paths:  []string{"app/.*\\.go", "server/.*\\.go"},
			        ChangedFiles:   []string{"model/order.go"},
			    },
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
			false,
			[]*pull.File{},
			&common.PredicateInfo{
		        Type:     "ModifiedLines",
		        Name:     "Modified Lines",
			    FileInfo: &common.FileInfo{
	                AdditionLimit:  "> 100",
	                DeletionLimit:  "> 10",
	                AddedLines:     0,
	                DeletedLines:   0,
			    },
			},
		},
		{
			"additions",
			true,
			[]*pull.File{
				{Additions: 55},
				{Additions: 10},
				{Additions: 45},
			},
			&common.PredicateInfo{
		        Type:     "ModifiedLines",
		        Name:     "Modified Lines",
			    FileInfo: &common.FileInfo{
	                AdditionLimit:  "> 100",
	                AddedLines:     110,
			    },
			},
		},
		{
			"deletions",
			true,
			[]*pull.File{
				{Additions: 5},
				{Additions: 10, Deletions: 10},
				{Additions: 5},
				{Deletions: 10},
			},
			&common.PredicateInfo{
		        Type:     "ModifiedLines",
		        Name:     "Modified Lines",
			    FileInfo: &common.FileInfo{
	                DeletionLimit:  "> 10",
	                DeletedLines:   20,
			    },
			},
		},
	})

	p = &ModifiedLines{
		Total: ComparisonExpr{Op: OpGreaterThan, Value: 100},
	}

	runFileTests(t, p, []FileTestCase{
		{
			"total",
			true,
			[]*pull.File{
				{Additions: 20, Deletions: 20},
				{Additions: 20},
				{Deletions: 20},
				{Additions: 20, Deletions: 20},
			},
			&common.PredicateInfo{
		        Type:     "ModifiedLines",
		        Name:     "Modified Lines",
			    FileInfo: &common.FileInfo{
	                TotalLimit:  "> 100",
                    TotalModifiedLines: 120,
			    },
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
	Name     string
	Expected bool
	Files    []*pull.File
	ExpectedPredicateInfo *common.PredicateInfo
}

func runFileTests(t *testing.T, p Predicate, cases []FileTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			prctx := &pulltest.Context{
				ChangedFilesValue: tc.Files,
			}

			ok, _, predicateInfo, err := p.Evaluate(ctx, prctx)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.Expected, ok, "predicate was not correct")
				assert.Equal(t, *tc.ExpectedPredicateInfo.FileInfo, *predicateInfo.FileInfo, "FileInfo was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Name, predicateInfo.Name, "PredicateInfo's Name was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Type, predicateInfo.Type, "PredicateInfo's Type was not correct")
			}
		})
	}
}
