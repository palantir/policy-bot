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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/palantir/policy-bot/policy/common"
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

func mockCheckRun(status string, conclusion string) *github.CheckRun {
	id := int64(1)
	nodeID := "node-id-123"
	headSHA := "abc123"
	externalID := "external-id"
	url := "https://api.github.com/repos/test/Hello-World/check-runs/1"
	htmlURL := "https://github.com/test/Hello-World/check-runs/1"
	detailsURL := "https://github.com/test/Hello-World/check-runs/1/details"
	startedAt := github.Timestamp{Time: time.Now()}
	completedAt := github.Timestamp{Time: time.Now()}
	name := "test-check-run"
	checkSuite := &github.CheckSuite{
		ID: github.Ptr(int64(1)),
	}
	app := &github.App{
		ID: github.Ptr(int64(1)),
	}
	pullRequests := []*github.PullRequest{
		{
			ID: github.Ptr(int64(1)),
		},
	}

	return &github.CheckRun{
		ID:           &id,
		NodeID:       &nodeID,
		HeadSHA:      &headSHA,
		ExternalID:   &externalID,
		URL:          &url,
		HTMLURL:      &htmlURL,
		DetailsURL:   &detailsURL,
		Status:       &status,
		Conclusion:   &conclusion,
		StartedAt:    &startedAt,
		CompletedAt:  &completedAt,
		Name:         &name,
		CheckSuite:   checkSuite,
		App:          app,
		PullRequests: pullRequests,
	}
}

func mockRepoStatus(state string) *github.RepoStatus {
	id := int64(1)
	nodeID := "node-id-123"
	url := "https://api.github.com/repos/octocat/Hello-World/statuses/1"
	targetURL := "https://github.com/octocat/Hello-World/statuses/1"
	context := "default"
	avatarURL := "https://avatars.githubusercontent.com/u/1234?v=4"
	createdAt := github.Timestamp{Time: time.Now()}
	updatedAt := github.Timestamp{Time: time.Now()}

	return &github.RepoStatus{
		ID:          &id,
		NodeID:      &nodeID,
		URL:         &url,
		State:       &state,
		TargetURL:   &targetURL,
		Description: nil,
		Context:     &context,
		AvatarURL:   &avatarURL,
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
}

func TestHasSuccessfulStatusCheck(t *testing.T) {
	hasStatusCheck := HasStatusCheck{Checks: []common.Regexp{
		common.NewMustCompileRegexp("status-name"),
		common.NewMustCompileRegexp("status-name-2"),
	}}
	hasStatusCheckSkippedOk := HasStatusCheck{
		Checks: []common.Regexp{
			common.NewMustCompileRegexp("status-name"),
			common.NewMustCompileRegexp("status-name-2"),
		},
		Conclusions: AllowedConclusions{"success", "skipped"},
	}

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

	testSuites := []StatusTestSuite{
		{predicate: hasStatusCheck, testCases: commonTestCases},
		{predicate: hasStatusCheck, testCases: okOnlyIfSkippedAllowed},
		{
			nameSuffix: "skipped allowed",
			predicate:  hasStatusCheckSkippedOk,
			testCases:  commonTestCases,
		},
		{
			nameSuffix:        "skipped allowed",
			predicate:         hasStatusCheckSkippedOk,
			testCases:         okOnlyIfSkippedAllowed,
			overrideSatisfied: github.Bool(true),
		},
	}

	for _, suite := range testSuites {
		runStatusTestCase(t, suite.predicate, suite)
	}
}

func runStatusCheckTestCase(t *testing.T, p Predicate, suite StatusTestSuite) {
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

		// `predicate.HasStatusCheck` -> `HasStatusCheck`
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
