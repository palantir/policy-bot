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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestCommitCount(t *testing.T) {
	ctx := context.Background()

	t.Run("lessThan", func(t *testing.T) {
		p := &CommitCount{
			Total: ComparisonExpr{Op: OpLessThan, Value: 5},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123"},
				{SHA: "def456"},
				{SHA: "ghi789"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "3 commits should be < 5")
			assert.Equal(t, []string{"3"}, result.Values)
		}
	})

	t.Run("greaterThan", func(t *testing.T) {
		p := &CommitCount{
			Total: ComparisonExpr{Op: OpGreaterThan, Value: 2},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123"},
				{SHA: "def456"},
				{SHA: "ghi789"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "3 commits should be > 2")
			assert.Equal(t, []string{"3"}, result.Values)
		}
	})

	t.Run("equals", func(t *testing.T) {
		p := &CommitCount{
			Total: ComparisonExpr{Op: OpEquals, Value: 3},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123"},
				{SHA: "def456"},
				{SHA: "ghi789"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "3 commits should = 3")
			assert.Equal(t, []string{"3"}, result.Values)
		}
	})

	t.Run("notSatisfied", func(t *testing.T) {
		p := &CommitCount{
			Total: ComparisonExpr{Op: OpLessThan, Value: 2},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123"},
				{SHA: "def456"},
				{SHA: "ghi789"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "3 commits should not be < 2")
			assert.Equal(t, []string{"3"}, result.Values)
		}
	})

	t.Run("zeroCommits", func(t *testing.T) {
		p := &CommitCount{
			Total: ComparisonExpr{Op: OpEquals, Value: 0},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "0 commits should = 0")
			assert.Equal(t, []string{"0"}, result.Values)
		}
	})

	t.Run("noConditions", func(t *testing.T) {
		p := &CommitCount{}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "should be unsatisfied when no conditions specified")
		}
	})
}

func TestCommitCountTrigger(t *testing.T) {
	p := &CommitCount{
		Total: ComparisonExpr{Op: OpLessThan, Value: 10},
	}

	assert.Equal(t, common.TriggerCommit, p.Trigger())
}
