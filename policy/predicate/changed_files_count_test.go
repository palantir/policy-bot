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

func TestChangedFilesCount(t *testing.T) {
	ctx := context.Background()

	t.Run("totalCount", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total: ComparisonExpr{Op: OpLessThan, Value: 5},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileAdded},
				{Filename: "file2.go", Status: pull.FileModified},
				{Filename: "file3.go", Status: pull.FileDeleted},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "3 files should be < 5")
			assert.Contains(t, result.Values, "total 3")
		}
	})

	t.Run("addedCount", func(t *testing.T) {
		p := &ChangedFilesCount{
			Added: ComparisonExpr{Op: OpLessThan, Value: 3},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileAdded},
				{Filename: "file2.go", Status: pull.FileAdded},
				{Filename: "file3.go", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "2 added files should be < 3")
			assert.Contains(t, result.Values, "added 2")
		}
	})

	t.Run("modifiedCount", func(t *testing.T) {
		p := &ChangedFilesCount{
			Modified: ComparisonExpr{Op: OpEquals, Value: 2},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileModified},
				{Filename: "file2.go", Status: pull.FileModified},
				{Filename: "file3.go", Status: pull.FileAdded},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "2 modified files should = 2")
			assert.Contains(t, result.Values, "modified 2")
		}
	})

	t.Run("deletedCount", func(t *testing.T) {
		p := &ChangedFilesCount{
			Deleted: ComparisonExpr{Op: OpGreaterThan, Value: 0},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileDeleted},
				{Filename: "file2.go", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "1 deleted file should be > 0")
			assert.Contains(t, result.Values, "deleted 1")
		}
	})

	t.Run("withIncludeFilter", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total: ComparisonExpr{Op: OpLessThan, Value: 3},
			Files: ChangedFilesCountFileFilter{
				Include: []common.Regexp{
					common.NewCompiledRegexp(regexp.MustCompile("^src/.*")),
				},
			},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "src/file1.go", Status: pull.FileAdded},
				{Filename: "src/file2.go", Status: pull.FileModified},
				{Filename: "docs/readme.md", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "2 files in src/ should be < 3")
			assert.Contains(t, result.Values, "total 2")
		}
	})

	t.Run("withExcludeFilter", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total: ComparisonExpr{Op: OpEquals, Value: 2},
			Files: ChangedFilesCountFileFilter{
				Exclude: []common.Regexp{
					common.NewCompiledRegexp(regexp.MustCompile(`.*\.md$`)),
				},
			},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileAdded},
				{Filename: "file2.go", Status: pull.FileModified},
				{Filename: "readme.md", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "2 non-md files should = 2")
			assert.Contains(t, result.Values, "total 2")
		}
	})

	t.Run("withIncludeAndExclude", func(t *testing.T) {
		p := &ChangedFilesCount{
			Added: ComparisonExpr{Op: OpEquals, Value: 1},
			Files: ChangedFilesCountFileFilter{
				Include: []common.Regexp{
					common.NewCompiledRegexp(regexp.MustCompile("^src/.*")),
				},
				Exclude: []common.Regexp{
					common.NewCompiledRegexp(regexp.MustCompile(`.*_test\.go$`)),
				},
			},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "src/file1.go", Status: pull.FileAdded},
				{Filename: "src/file1_test.go", Status: pull.FileAdded},
				{Filename: "docs/readme.md", Status: pull.FileAdded},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "1 added file in src/ (excluding tests) should = 1")
			assert.Contains(t, result.Values, "added 1")
		}
	})

	t.Run("multipleConditionsFirstSatisfied", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total:    ComparisonExpr{Op: OpLessThan, Value: 5},
			Added:    ComparisonExpr{Op: OpGreaterThan, Value: 10},
			Modified: ComparisonExpr{Op: OpEquals, Value: 2},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileAdded},
				{Filename: "file2.go", Status: pull.FileModified},
				{Filename: "file3.go", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "total condition satisfied first")
			assert.Contains(t, result.Values, "total 3")
		}
	})

	t.Run("noneConditionsSatisfied", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total: ComparisonExpr{Op: OpGreaterThan, Value: 10},
			Added: ComparisonExpr{Op: OpGreaterThan, Value: 5},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{
				{Filename: "file1.go", Status: pull.FileAdded},
				{Filename: "file2.go", Status: pull.FileModified},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "no conditions should be satisfied")
			assert.Contains(t, result.Values, "total 2")
			assert.Contains(t, result.Values, "added 1")
		}
	})

	t.Run("zeroFiles", func(t *testing.T) {
		p := &ChangedFilesCount{
			Total: ComparisonExpr{Op: OpEquals, Value: 0},
		}

		prctx := &pulltest.Context{
			ChangedFilesValue: []*pull.File{},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "0 files should = 0")
			assert.Contains(t, result.Values, "total 0")
		}
	})
}

func TestChangedFilesCountTrigger(t *testing.T) {
	p := &ChangedFilesCount{
		Total: ComparisonExpr{Op: OpLessThan, Value: 20},
	}

	assert.Equal(t, common.TriggerCommit, p.Trigger())
}
