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

package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

type StaticEvaluator common.Result

func (eval *StaticEvaluator) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return common.Result(*eval)
}

func TestEvaluator(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{}

	t.Run("disapprovalWins", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status: common.StatusApproved,
			},
			disapproval: &StaticEvaluator{
				Status:      common.StatusDisapproved,
				Description: "disapproved by test",
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, common.StatusDisapproved, r.Status)
		assert.Equal(t, "disapproved by test", r.Description)
	})

	t.Run("approvalWinsByDefault", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status:      common.StatusPending,
				Description: "2 approvals needed",
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusSkipped,
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, common.StatusPending, r.Status)
		assert.Equal(t, "2 approvals needed", r.Description)
	})

	t.Run("propagateError", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Error: errors.New("approval failed"),
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusDisapproved,
			},
		}

		r := eval.Evaluate(ctx, prctx)

		assert.EqualError(t, r.Error, "approval failed")
		assert.Equal(t, common.StatusSkipped, r.Status)
	})

	t.Run("setsProperties", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status: common.StatusPending,
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusDisapproved,
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, "policy", r.Name)
		if assert.Len(t, r.Children, 2) {
			assert.Equal(t, castToResult(eval.approval), r.Children[0])
			assert.Equal(t, castToResult(eval.disapproval), r.Children[1])
		}
	})
}

func castToResult(e common.Evaluator) *common.Result {
	return (*common.Result)(e.(*StaticEvaluator))
}
