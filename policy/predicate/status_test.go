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
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-github/v70/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestHasSuccessfulStatus(t *testing.T) {
	hasStatus := HasStatus{Statuses: []string{"status-name", "status-name-2"}}
	hasRegexStatus := HasStatus{Statuses: []string{".*"}}
	hasStatusSkippedOk := HasStatus{
		Statuses:    []string{"status-name", "status-name-2"},
		Conclusions: []string{"success", "skipped"},
	}
	hasSuccessfulStatus := HasSuccessfulStatus{"status-name", "status-name-2"}
	hasSuccessfulRegexStatus := HasSuccessfulStatus{".*"}

	commonTestCases := []StatusTestCase{
		{
			"all statuses succeed",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("completed", "success"),
				},
			},
			&common.PredicateResult{
				Satisfied: true,
			},
		},
		{
			"a status fails",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("completed", "failure"),
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
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "failure"),
					"status-name-2": mockCheckRun("completed", "failure"),
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
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name": mockCheckRun("completed", "success"),
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
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name-2": mockCheckRun("completed", "skipped"),
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
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("completed", "skipped"),
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
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "skipped"),
					"status-name-2": mockCheckRun("completed", "skipped"),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
	}

	nonCompleteStatusesTestCases := []StatusTestCase{
		{
			"a workflow in state 'expected', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("expected", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'failure', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("failure", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'in_progress', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("in_progress", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'queued', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("queued", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'pending', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("pending", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'waiting', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("waiting", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'requested', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("requested", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			"a workflow in state 'startup_failure', it should be threaded as a failure",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("startup_failure", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
	}

	regexStatusTestCases := []StatusTestCase{
		{
			"a predicate with regex syntax is treated as a literal string and not found",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("startup_failure", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{".*"},
			},
		},
	}

	regexSuccessfullStatusTestCases := []StatusTestCase{
		{
			"a predicate with regex syntax is treated as a literal string and not found",
			&pulltest.Context{
				LatestCheckStatusesValue: map[string]*github.CheckRun{
					"status-name":   mockCheckRun("completed", "success"),
					"status-name-2": mockCheckRun("startup_failure", ""),
				},
			},
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{".*"},
			},
		},
	}

	testSuites := []StatusTestSuite{
		{predicate: hasStatus, testCases: commonTestCases},
		{predicate: hasStatus, testCases: okOnlyIfSkippedAllowed},
		{predicate: hasSuccessfulStatus, testCases: commonTestCases},
		{predicate: hasSuccessfulStatus, testCases: okOnlyIfSkippedAllowed},
		{predicate: hasStatus, testCases: nonCompleteStatusesTestCases},
		{predicate: hasRegexStatus, testCases: regexStatusTestCases},
		{predicate: hasSuccessfulRegexStatus, testCases: regexSuccessfullStatusTestCases},
		{
			nameSuffix: "skipped allowed",
			predicate:  hasStatusSkippedOk,
			testCases:  commonTestCases,
		},
		{
			nameSuffix:        "skipped allowed",
			predicate:         hasStatusSkippedOk,
			testCases:         okOnlyIfSkippedAllowed,
			overrideSatisfied: github.Bool(true),
		},
	}

	for _, suite := range testSuites {
		runStatusTestCase(t, suite.predicate, suite)
	}
}

type StatusTestSuite struct {
	nameSuffix        string
	predicate         Predicate
	testCases         []StatusTestCase
	overrideSatisfied *bool
}

type StatusTestCase struct {
	name                    string
	context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runStatusTestCase(t *testing.T, p Predicate, suite StatusTestSuite) {
	ctx := context.Background()

	for _, tc := range suite.testCases {
		if suite.overrideSatisfied != nil {
			tc.ExpectedPredicateResult.Satisfied = *suite.overrideSatisfied
		}

		// If the test case expects the predicate to be satisfied, we always
		// expect the values to contain all the statuses. Doing this here lets
		// us use the same testcases when we allow and don't allow skipped
		// statuses.
		if tc.ExpectedPredicateResult.Satisfied {
			statuses, _ := tc.context.LatestCheckStatuses()
			tc.ExpectedPredicateResult.Values = keysSorted(statuses)
		}

		// `predicate.HasStatus` -> `HasStatus`
		_, predicateType, _ := strings.Cut(fmt.Sprintf("%T", p), ".")
		testName := fmt.Sprintf("%s_%s", predicateType, tc.name)

		if suite.nameSuffix != "" {
			testName += "_" + suite.nameSuffix
		}

		t.Run(testName, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}
