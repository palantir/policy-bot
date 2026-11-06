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

package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigSuppressesStatusForUnseenPolicy(t *testing.T) {
	ec := makeEvalContext(false)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	assert.Nil(t, ec.Status)
}

func TestParseConfigPostsStatusForSeenPolicy(t *testing.T) {
	ec := makeEvalContext(true)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	require.NotNil(t, ec.Status)
	assert.Equal(t, "error", ec.Status.GetState())
	assert.Equal(t, "policy-bot: main", ec.Status.GetContext())
	assert.Equal(t, "Error loading policy from testorg/testrepo@main", ec.Status.GetDescription())
	assert.Equal(t, []string{"policy-bot: main"}, ec.StatusContexts)
}

type fakeRulesetChecks struct {
	checks []string
	err    error
	calls  int
}

func (f *fakeRulesetChecks) RequiredChecks(ctx context.Context, client *github.Client, owner, repo, branch string) ([]string, error) {
	f.calls++
	return f.checks, f.err
}

type fakeEvaluator struct {
	status common.EvaluationStatus
}

func (e fakeEvaluator) Trigger() common.Trigger { return common.TriggerAll }

func (e fakeEvaluator) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return common.Result{Status: e.status, StatusDescription: "evaluated"}
}

func statusContextsOf(statuses []github.RepoStatus) []string {
	var contexts []string
	for _, s := range statuses {
		contexts = append(contexts, s.GetContext())
	}
	return contexts
}

func TestPostStatus(t *testing.T) {
	tests := map[string]struct {
		StatusChecks []StatusCheckType
		Rulesets     *fakeRulesetChecks
		Expected     []string
	}{
		"branch": {
			StatusChecks: []StatusCheckType{StatusCheckBranch},
			Rulesets:     &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}},
			Expected:     []string{"policy-bot: main"},
		},
		"unset": {
			Rulesets: &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}},
			Expected: []string{"policy-bot: main"},
		},
		"branchAndRuleset": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			Rulesets:     &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release", "policy-bot: ruleset:shared"}},
			Expected:     []string{"policy-bot: main", "policy-bot: ruleset:release", "policy-bot: ruleset:shared"},
		},
		"rulesetWithoutRulesets": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			Rulesets:     &fakeRulesetChecks{},
		},
		"deduplicated": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset, StatusCheckBranch},
			Rulesets:     &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release", "policy-bot: ruleset:release"}},
			Expected:     []string{"policy-bot: main", "policy-bot: ruleset:release"},
		},
		"rulesetLookupError": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			Rulesets:     &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}, err: errors.New("lookup failed")},
			Expected:     []string{"policy-bot: main"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)

			ec := makeEvalContext(true)
			ec.SkipPostStatus = false
			ec.Client = gh.Client()
			ec.Options.StatusChecks = test.StatusChecks
			ec.RulesetChecks = test.Rulesets

			registry := metrics.NewRegistry()
			ec.PostStatus(baseapp.WithMetricsCtx(context.Background(), registry), "success", "approved")

			statuses := gh.Statuses()
			assert.Equal(t, test.Expected, statusContextsOf(statuses))
			for _, s := range statuses {
				assert.Equal(t, "success", s.GetState())
				assert.Equal(t, "approved", s.GetDescription())
				assert.Equal(t, "https://policy-bot.example.com/details/testorg/testrepo/42", s.GetTargetURL())
			}
			for _, r := range gh.Requests() {
				assert.Equal(t, "POST /repos/testorg/testrepo/statuses/abc123", r)
			}

			var expectedRulesets int64
			for _, c := range test.Expected {
				if c != "policy-bot: main" {
					expectedRulesets++
				}
			}
			assert.Equal(t, expectedRulesets, metrics.GetOrRegisterCounter(MetricsKeyRulesetChecksPosted, registry).Count())
		})
	}
}

func TestPostStatusSkipPostStatus(t *testing.T) {
	ec := makeEvalContext(true)
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckRuleset}

	t.Run("rulesets", func(t *testing.T) {
		ec.RulesetChecks = &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:a", "policy-bot: ruleset:b"}}
		ec.PostStatus(context.Background(), "pending", "waiting")

		require.NotNil(t, ec.Status)
		assert.Equal(t, "policy-bot: ruleset:a", ec.Status.GetContext())
		assert.Equal(t, "pending", ec.Status.GetState())
		assert.Equal(t, []string{"policy-bot: ruleset:a", "policy-bot: ruleset:b"}, ec.StatusContexts)
	})

	t.Run("noContexts", func(t *testing.T) {
		ec.RulesetChecks = &fakeRulesetChecks{}
		ec.PostStatus(context.Background(), "pending", "waiting")

		require.NotNil(t, ec.Status)
		assert.Nil(t, ec.Status.Context)
		assert.Empty(t, ec.StatusContexts)
	})
}

func TestPostStatusClosedPullRequest(t *testing.T) {
	gh := newFakeGitHub(t)
	rulesets := &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}}

	ec := makeEvalContext(true)
	ec.SkipPostStatus = false
	ec.Client = gh.Client()
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckBranch, StatusCheckRuleset}
	ec.RulesetChecks = rulesets
	ec.PullContext.(*pulltest.Context).StateValue = "closed"

	ec.PostStatus(context.Background(), "success", "approved")

	assert.Empty(t, gh.Requests())
	assert.Zero(t, rulesets.calls, "rulesets should not be looked up for closed pull requests")
}

func TestPostStatusRulesetLookupError(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const combinedPath = "/repos/testorg/testrepo/commits/abc123/status"

	combinedStatus := func(contexts ...string) map[string]any {
		var statuses []map[string]any
		for _, c := range contexts {
			statuses = append(statuses, map[string]any{"context": c, "state": "success"})
		}
		return map[string]any{"state": "success", "statuses": statuses}
	}

	tests := map[string]struct {
		StatusChecks []StatusCheckType
		State        string
		Combined     map[int]fakeResponse // by page
		Expected     []string
	}{
		"successPostsOnlyBranch": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "success",
			Combined:     map[int]fakeResponse{0: {body: combinedStatus("policy-bot: ruleset:a")}},
			Expected:     []string{"policy-bot: main"},
		},
		"successRulesetOnlyPostsNothing": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "success",
			Combined:     map[int]fakeResponse{0: {body: combinedStatus("policy-bot: ruleset:a")}},
		},
		"pendingDowngradesPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "pending",
			Combined: map[int]fakeResponse{0: {body: combinedStatus(
				"ci/build",
				"policy-bot: main",
				"policy-bot: ruleset:b",
				"policy-bot: ruleset:a",
				"other-bot: ruleset:c",
			)}},
			Expected: []string{"policy-bot: main", "policy-bot: ruleset:a", "policy-bot: ruleset:b"},
		},
		"failureRulesetOnlyDowngradesPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "failure",
			Combined:     map[int]fakeResponse{0: {body: combinedStatus("policy-bot: ruleset:a")}},
			Expected:     []string{"policy-bot: ruleset:a"},
		},
		"errorDowngradesAllPages": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "error",
			Combined: map[int]fakeResponse{
				0: {body: combinedStatus("policy-bot: ruleset:a"), nextPage: 2},
				2: {body: combinedStatus("policy-bot: ruleset:b")},
			},
			Expected: []string{"policy-bot: ruleset:a", "policy-bot: ruleset:b"},
		},
		"noPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "failure",
			Combined:     map[int]fakeResponse{0: {body: combinedStatus("ci/build")}},
		},
		"combinedStatusError": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "failure",
			Combined:     map[int]fakeResponse{0: {status: http.StatusInternalServerError, body: map[string]any{}}},
			Expected:     []string{"policy-bot: main"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)
			gh.Respond(rulesPath, 0, fakeResponse{status: http.StatusInternalServerError, body: map[string]any{}})
			for page, res := range test.Combined {
				gh.Respond(combinedPath, page, res)
			}

			ec := makeEvalContext(true)
			ec.SkipPostStatus = false
			ec.Client = gh.Client()
			ec.Options.StatusChecks = test.StatusChecks
			ec.RulesetChecks = newTestFinder(t)

			ec.PostStatus(context.Background(), test.State, "message")

			statuses := gh.Statuses()
			assert.Equal(t, test.Expected, statusContextsOf(statuses))
			for _, s := range statuses {
				assert.Equal(t, test.State, s.GetState())
			}

			requests := gh.Requests()
			require.GreaterOrEqual(t, len(requests), 2)
			assert.Equal(t, []string{"GET " + rulesPath, "GET " + rulesPath}, requests[:2], "rules lookup should be retried once")
			if test.State == "success" {
				assert.NotContains(t, requests, "GET "+combinedPath, "success should not read previous statuses")
			}
		})
	}
}

func TestPostStatusRulesetLookupErrorSkipPostStatus(t *testing.T) {
	gh := newFakeGitHub(t)

	ec := makeEvalContext(true)
	ec.Client = gh.Client()
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckBranch, StatusCheckRuleset}
	ec.RulesetChecks = &fakeRulesetChecks{err: errors.New("lookup failed")}

	ec.PostStatus(context.Background(), "failure", "message")

	assert.Equal(t, []string{"policy-bot: main"}, ec.StatusContexts)
	assert.Empty(t, gh.Requests(), "details evaluations should not read previous statuses")
}

func TestEvaluateWithoutStatusContexts(t *testing.T) {
	gh := newFakeGitHub(t)

	ec := makeEvalContext(true)
	ec.SkipPostStatus = false
	ec.Client = gh.Client()
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckRuleset}
	ec.RulesetChecks = &fakeRulesetChecks{}

	result, err := ec.EvaluatePolicy(context.Background(), fakeEvaluator{status: common.StatusApproved})
	require.NoError(t, err)
	assert.Equal(t, common.StatusApproved, result.Status)
	assert.Equal(t, "evaluated", result.StatusDescription)
	assert.Empty(t, gh.Requests())
}

func makeEvalContext(seenPolicy bool) *EvalContext {
	return &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL: "https://policy-bot.example.com",
		PullContext: &pulltest.Context{
			OwnerValue:     "testorg",
			RepoValue:      "testrepo",
			NumberValue:    42,
			StateValue:     "open",
			HeadSHAValue:   "abc123",
			BranchBaseName: "main",
			BranchHeadName: "feature",
		},
		SkipPostStatus: true,
		Config: FetchedConfig{
			LoadError:  errors.New("request failed"),
			Source:     "testorg/testrepo@main",
			Path:       ".policy.yml",
			SeenPolicy: seenPolicy,
		},
	}
}
