// Copyright 2019 Palantir Technologies, Inc.
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

func TestHasSuccessfulStatus(t *testing.T) {
	p := HasSuccessfulStatus([]string{"status-name", "status-name-2"})

	runStatusTestCase(t, p, []StatusTestCase{
		{
			"all statuses succeed",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "success",
					"status-name-2": "success",
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			"a status fails",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "success",
					"status-name-2": "failure",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"multiple statuses fail",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "failure",
					"status-name-2": "failure",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			"a status does not exist",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name": "success",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"multiple statuses do not exist",
			&pulltest.Context{},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
	})
}

type StatusTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runStatusTestCase(t *testing.T, p Predicate, cases []StatusTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
