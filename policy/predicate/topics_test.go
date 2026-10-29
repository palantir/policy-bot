// Copyright 2026 Palantir Technologies, Inc.
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
	"errors"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestHasTopics(t *testing.T) {
	p := HasTopics([]string{"lib", "go"})

	runTopicsTestCase(t, p, []HasTopicsTestCase{
		{
			"all topics",
			&pulltest.Context{
				RepositoryTopicsValue: []string{"lib", "go"},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"lib", "go"},
				ConditionValues: []string{"lib", "go"},
			},
		},
		{
			"missing a topic",
			&pulltest.Context{
				RepositoryTopicsValue: []string{"lib"},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"lib"},
				ConditionValues: []string{"go"},
			},
		},
		{
			"no topics",
			&pulltest.Context{
				RepositoryTopicsValue: []string{},
			},
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{},
				ConditionValues: []string{"lib"},
			},
		},
		{
			"error from context",
			&pulltest.Context{
				RepositoryTopicsError: errors.New("api error"),
			},
			nil,
		},
	})
}

func TestHasTopicsEmpty(t *testing.T) {
	p := HasTopics([]string{})

	runTopicsTestCase(t, p, []HasTopicsTestCase{
		{
			"empty predicate is satisfied",
			&pulltest.Context{
				RepositoryTopicsValue: []string{"lib"},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          nil,
				ConditionValues: []string{},
			},
		},
	})
}

func TestHasTopicsMixedCase(t *testing.T) {
	p := HasTopics([]string{"Lib", "Go"})

	runTopicsTestCase(t, p, []HasTopicsTestCase{
		{
			"mixed case requirement matches lowercase topics",
			&pulltest.Context{
				RepositoryTopicsValue: []string{"lib", "go"},
			},
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"lib", "go"},
				ConditionValues: []string{"Lib", "Go"},
			},
		},
	})
}

func TestHasTopicsTrigger(t *testing.T) {
	p := HasTopics([]string{"lib"})
	assert.Equal(t, common.TriggerStatic, p.Trigger())
}

type HasTopicsTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runTopicsTestCase(t *testing.T, p Predicate, cases []HasTopicsTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if tc.ExpectedPredicateResult == nil {
				assert.Error(t, err)
				return
			}
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
