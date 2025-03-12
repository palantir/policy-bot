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

type StatusCheckTestCase struct {
	name                     string
	LatestCheckStatusesValue map[string]*github.CheckRun
	LatestRepoStatusesValue  map[string]*github.RepoStatus
	predicate                Predicate
	ExpectedPredicateResult  *common.PredicateResult
}

func runStatusCheckTestCase(t *testing.T, cases []StatusCheckTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			predicateResult, err := tc.predicate.Evaluate(ctx, &pulltest.Context{
				LatestCheckStatusesValue: tc.LatestCheckStatusesValue,
				LatestRepoStatusesValue:  tc.LatestRepoStatusesValue,
			})
			if assert.NoError(t, err, "evaluation failed") {
				assertPredicateResult(t, tc.ExpectedPredicateResult, predicateResult)
			}
		})
	}
}

func TestHasStatusCheck(t *testing.T) {
	defaultChecksTestCases := []StatusCheckTestCase{
		{
			name: "all status check succeed",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "a status check fails",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "failure"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			name: "multiple status checks fail",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "failure"),
				"status-name-2": mockCheckRun("completed", "failure"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "a status check does not exist",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name": mockCheckRun("completed", "success"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp("test.*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test.*"},
			},
		},
		{
			name: "a status check does not exist, the other status check is skipped",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"test": mockCheckRun("completed", "skipped"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test"},
			},
		},
		{
			name:                     "multiple status checks do not exist",
			LatestCheckStatusesValue: map[string]*github.CheckRun{},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".*"},
			},
		},
		{
			name: "a status check has non completed status",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"test":  mockCheckRun("completed", "success"),
				"test1": mockCheckRun("expected", ""),
				"test2": mockCheckRun("failure", ""),
				"test3": mockCheckRun("in_progress", ""),
				"test4": mockCheckRun("pending", ""),
				"test5": mockCheckRun("queued", ""),
				"test6": mockCheckRun("requested", ""),
				"test7": mockCheckRun("startup_failure", ""),
				"test8": mockCheckRun("waiting", ""),
				"test9": mockCheckRun("invalid_state", ""),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test2", "test3", "test4", "test5", "test6", "test7", "test8", "test9"},
			},
		},
		{
			name: "status checks is have status completed but non successfull",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"test":  mockCheckRun("completed", "success"),
				"test1": mockCheckRun("completed", "action_required"),
				"test2": mockCheckRun("completed", "cancelled"),
				"test3": mockCheckRun("completed", "failure"),
				"test4": mockCheckRun("completed", "neutral"),
				"test5": mockCheckRun("completed", "skipped"),
				"test6": mockCheckRun("completed", "stale"),
				"test7": mockCheckRun("completed", "timed_out"),
				"test8": mockCheckRun("completed", "invalid_conclusion"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test2", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
	}

	allConclusionChecksValue := map[string]*github.CheckRun{
		"test":  mockCheckRun("completed", "success"),
		"test1": mockCheckRun("completed", "action_required"),
		"test2": mockCheckRun("completed", "cancelled"),
		"test3": mockCheckRun("completed", "failure"),
		"test4": mockCheckRun("completed", "neutral"),
		"test5": mockCheckRun("completed", "skipped"),
		"test6": mockCheckRun("completed", "stale"),
		"test7": mockCheckRun("completed", "timed_out"),
		"test8": mockCheckRun("completed", "invalid_conclusion"),
	}
	customConclusionChecksTestCases := []StatusCheckTestCase{
		{
			name:                     "status checks is have status completed but conclusion action_required is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"action_required"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test2", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion cancelled is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"cancelled"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion failure is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion neutral is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"neutral"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion skipped is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"skipped"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion stale is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"stale"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test5", "test7", "test8"},
			},
		},
		{
			name:                     "status checks is have status completed but conclusion timed_out is allowed",
			LatestCheckStatusesValue: allConclusionChecksValue,
			predicate: HasStatusCheck{
				Checks:      []common.Regexp{common.NewMustCompileRegexp(".*")},
				Conclusions: []string{"timed_out"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test5", "test6", "test8"},
			},
		},
	}

	allStatusesChecksValue := map[string]*github.CheckRun{
		"test":  mockCheckRun("completed", "success"),
		"test1": mockCheckRun("expected", ""),
		"test2": mockCheckRun("failure", ""),
		"test3": mockCheckRun("in_progress", ""),
		"test4": mockCheckRun("pending", ""),
		"test5": mockCheckRun("queued", ""),
		"test6": mockCheckRun("requested", ""),
		"test7": mockCheckRun("startup_failure", ""),
		"test8": mockCheckRun("waiting", ""),
	}
	customStatusCheckTestCases := []StatusCheckTestCase{
		{
			name:                     "status checks exist with all possible states but status completed is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"completed"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test2", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status expected is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"expected"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test2", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status failure is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test3", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status in_progress is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"in_progress"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test4", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status pending is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"pending"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test5", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status queued is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"queued"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test6", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but requested is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"requested"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test5", "test7", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status startup_failure is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"startup_failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test5", "test6", "test8"},
			},
		},
		{
			name:                     "status checks exist with all possible states but status waiting is allowed",
			LatestCheckStatusesValue: allStatusesChecksValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"waiting"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test", "test1", "test2", "test3", "test4", "test5", "test6", "test7"},
			},
		},
	}

	allStatusesRepoStatusValue := map[string]*github.RepoStatus{
		"test1": mockRepoStatus("success"),
		"test2": mockRepoStatus("error"),
		"test3": mockRepoStatus("pending"),
		"test4": mockRepoStatus("failure"),
	}
	defaultRepoStatusTestCases := []StatusCheckTestCase{
		{
			name: "all repo statuse succeed",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"status-name":   mockRepoStatus("success"),
				"status-name-2": mockRepoStatus("success"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "a repo statuse fails",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"status-name":   mockRepoStatus("success"),
				"status-name-2": mockRepoStatus("failure"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name-2"},
			},
		},
		{
			name: "multiple repo statuses fail",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"status-name":   mockRepoStatus("failure"),
				"status-name-2": mockRepoStatus("failure"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "a repo status does not exist",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"status-name": mockRepoStatus("success"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp("test.*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test.*"},
			},
		},
		{
			name: "a repo status does not exist, the other repo status is pending",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"test": mockRepoStatus("pending"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test"},
			},
		},
		{
			name: "a repo status does not exist, the other repo status is error",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{
				"test": mockRepoStatus("error"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test"},
			},
		},
		{
			name:                    "multiple repo statuses do not exist",
			LatestRepoStatusesValue: map[string]*github.RepoStatus{},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{".*"},
			},
		},
		{
			name:                    "a repo status has non success status",
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test2", "test3", "test4"},
			},
		},
	}

	customStatusRepoStatusTestCases := []StatusCheckTestCase{
		{
			name:                    "repo statuses exist with all possible states but status success is allowed",
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"success"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test2", "test3", "test4"},
			},
		},
		{
			name:                    "repo statuses exist with all possible states but status error is allowed",
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"error"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test3", "test4"},
			},
		},
		{
			name:                    "repo statuses exist with all possible states but status pending is allowed",
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"pending"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test2", "test4"},
			},
		},
		{
			name:                    "repo statuses exist with all possible states but status failure is allowed",
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks:   []common.Regexp{common.NewMustCompileRegexp(".*")},
				Statuses: []string{"failure"},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"test1", "test2", "test3"},
			},
		},
	}

	regexTestCases := []StatusCheckTestCase{
		{
			name: "a predicate check is threaded as regex",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
				"abc":           mockCheckRun("completed", "failure"),
			},
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp("status-name.*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "a predicate check is threaded as literal string if noRegex is set negative test",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
				"abc":           mockCheckRun("completed", "failure"),
			},
			predicate: HasStatusCheck{
				Checks:  []common.Regexp{common.NewMustCompileRegexp("status-name.*")},
				noRegex: true,
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"status-name.*"},
			},
		},
		{
			name: "a predicate check is threaded as literal string if noRegex is set positive test",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
				"abc":           mockCheckRun("completed", "failure"),
			},
			predicate: HasStatusCheck{
				Checks:  []common.Regexp{common.NewMustCompileRegexp("status-name")},
				noRegex: true,
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name"},
			},
		},
	}

	checkAndRepoStatusMixedTestCases := []StatusCheckTestCase{
		{
			name: "A regex only matching statuses returns only matching statuses",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
				"abc":           mockCheckRun("completed", "failure"),
			},
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp("status-name.*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: true,
				Values:    []string{"status-name", "status-name-2"},
			},
		},
		{
			name: "A regex matching everything returns compleded checks and successful repo statuses by default",
			LatestCheckStatusesValue: map[string]*github.CheckRun{
				"status-name":   mockCheckRun("completed", "success"),
				"status-name-2": mockCheckRun("completed", "success"),
				"abc":           mockCheckRun("completed", "failure"),
			},
			LatestRepoStatusesValue: allStatusesRepoStatusValue,
			predicate: HasStatusCheck{
				Checks: []common.Regexp{common.NewMustCompileRegexp(".*")},
			},
			ExpectedPredicateResult: &common.PredicateResult{
				Satisfied: false,
				Values:    []string{"abc", "test2", "test3", "test4"},
			},
		},
	}

	runStatusCheckTestCase(t, defaultChecksTestCases)
	runStatusCheckTestCase(t, customConclusionChecksTestCases)
	runStatusCheckTestCase(t, customStatusCheckTestCases)
	runStatusCheckTestCase(t, defaultRepoStatusTestCases)
	runStatusCheckTestCase(t, customStatusRepoStatusTestCases)
	runStatusCheckTestCase(t, checkAndRepoStatusMixedTestCases)
	runStatusCheckTestCase(t, regexTestCases)
}

func TestjoinElementsWithOr(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			"empty",
			[]string{},
			"",
		},
		{
			"single",
			[]string{"a"},
			"a",
		},
		{
			"two",
			[]string{"a", "b"},
			"a or b",
		},
		{
			"three",
			[]string{"a", "b", "c"},
			"a, b, or c",
		},
		{
			"conclusions get sorted",
			[]string{"c", "a", "b"},
			"a, b, or c",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, joinElementsWithOr(tc.input))
		})
	}
}
