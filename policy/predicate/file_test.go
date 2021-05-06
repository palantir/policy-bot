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
		},
	})
}

func TestModifiedLines(t *testing.T) {
	p := &ModifiedLines{
		Additions: ">100",
		Deletions: ">10",
	}

	runFileTests(t, p, []FileTestCase{
		{
			"empty",
			false,
			[]*pull.File{},
		},
		{
			"additions",
			true,
			[]*pull.File{
				{Additions: 55},
				{Additions: 10},
				{Additions: 45},
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
		},
	})

	p = &ModifiedLines{
		Total: ">100",
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
		},
	})
}

func TestComparisonExpr(t *testing.T) {
	tests := map[string]struct {
		Expr   ComparisonExpr
		Value  int64
		Output bool
		Err    bool
	}{
		"greaterThanTrue": {
			Expr:   ">100",
			Value:  200,
			Output: true,
		},
		"greaterThanFalse": {
			Expr:   "> 100",
			Value:  50,
			Output: false,
		},
		"lessThanTrue": {
			Expr:   "<100",
			Value:  50,
			Output: true,
		},
		"lessThanFalse": {
			Expr:   "< 100",
			Value:  200,
			Output: false,
		},
		"invalidOperator": {
			Expr: "=200",
			Err:  true,
		},
		"invalidNumber": {
			Expr: ">12ab",
			Err:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ok, err := test.Expr.Evaluate(test.Value)
			if test.Err {
				assert.Error(t, err, "expected error evaluating expression, but got nil")
				return
			}
			if assert.NoError(t, err, "unexpected error evaluating expression") {
				assert.Equal(t, test.Output, ok, "evaluation was not correct")
			}
		})
	}

	t.Run("isEmpty", func(t *testing.T) {
		assert.True(t, ComparisonExpr("").IsEmpty(), "expression was not empty")
		assert.False(t, ComparisonExpr(">100").IsEmpty(), "expression was empty")
	})
}

type FileTestCase struct {
	Name     string
	Expected bool
	Files    []*pull.File
}

func runFileTests(t *testing.T, p Predicate, cases []FileTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			prctx := &pulltest.Context{
				ChangedFilesValue: tc.Files,
			}

			ok, _, err := p.Evaluate(ctx, prctx)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.Expected, ok, "predicate was not correct")
			}
		})
	}
}
