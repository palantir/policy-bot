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
	"slices"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func keysSorted[V any](m map[string]V) []string {
	r := make([]string, 0, len(m))

	for k := range m {
		r = append(r, k)
	}

	slices.Sort(r)
	return r
}

func TestHasSuccessfulStatus(t *testing.T) {
	p := HasSuccessfulStatus{Statuses: []string{"status-name", "status-name-2"}}

	commonTestCases := []StatusTestCase{
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
			"a status does not exist, the other status is skipped",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name-2": "skipped",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name"},
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
	}

	okOnlyIfSkippedAllowed := []StatusTestCase{
		{
			"a status is skipped",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "success",
					"status-name-2": "skipped",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"all statuses are skipped",
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "skipped",
					"status-name-2": "skipped",
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
	}

	// Run tests with skipped statuses counting as failures
	runStatusTestCase(t, p, commonTestCases)
	runStatusTestCase(t, p, okOnlyIfSkippedAllowed)

	// Run tests with skipped statuses counting as successes
	p.Options.SkippedIsSuccess = true

	for i := 0; i < len(commonTestCases); i++ {
		commonTestCases[i].name += ", but skipped statuses are allowed"
	}
	for i := 0; i < len(okOnlyIfSkippedAllowed); i++ {
		okOnlyIfSkippedAllowed[i].name += ", but skipped statuses are allowed"
		okOnlyIfSkippedAllowed[i].ExpectedPredicateResult.Satisfied = true
	}
	runStatusTestCase(t, p, commonTestCases)
	runStatusTestCase(t, p, okOnlyIfSkippedAllowed)
}

type StatusTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runStatusTestCase(t *testing.T, p Predicate, cases []StatusTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		// If the test case expects the predicate to be satisfied, we always
		// expect the values to contain all the statuses. Doing this here lets
		// us use the same testcases when we allow and don't allow skipped
		// statuses.
		if tc.ExpectedPredicateResult.Satisfied {
			statuses, _ := tc.context.LatestStatuses()
			tc.ExpectedPredicateResult.Values = keysSorted(statuses)
		}

		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
