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
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/internal/githubtest"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
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
	assert.Contains(t, ec.Status.GetDescription(), "testorg/testrepo@main")
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
			rp := &githubtest.ResponsePlayer{}
			statuses := recordStatuses(rp, statusesPath)

			ec := makeEvalContext(true)
			ec.SkipPostStatus = false
			ec.Client = newTestClient(t, rp)
			ec.Options.StatusChecks = test.StatusChecks
			ec.RulesetChecks = test.Rulesets

			ec.PostStatus(context.Background(), "success", "approved")

			assert.Equal(t, test.Expected, statuses.contexts())
			for _, s := range statuses.statuses {
				assert.Equal(t, "success", s.GetState())
				assert.Equal(t, "approved", s.GetDescription())
				assert.Equal(t, "https://policy-bot.example.com/details/testorg/testrepo/42", s.GetTargetURL())
			}
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
	rp := &githubtest.ResponsePlayer{}
	statuses := recordStatuses(rp, statusesPath)
	rulesets := &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}}

	ec := makeEvalContext(true)
	ec.SkipPostStatus = false
	ec.Client = newTestClient(t, rp)
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckBranch, StatusCheckRuleset}
	ec.RulesetChecks = rulesets
	ec.PullContext.(*pulltest.Context).StateValue = "closed"

	ec.PostStatus(context.Background(), "success", "approved")

	assert.Empty(t, statuses.statuses)
	assert.Zero(t, rulesets.calls, "rulesets should not be looked up for closed pull requests")
}

func TestPostStatusRulesetLookupError(t *testing.T) {
	const combinedPath = "/repos/testorg/testrepo/commits/abc123/status"

	tests := map[string]struct {
		StatusChecks []StatusCheckType
		State        string
		Combined     string
		Expected     []string
	}{
		"successPostsOnlyBranch": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "success",
			Combined:     "combined_status_ruleset_a.yml",
			Expected:     []string{"policy-bot: main"},
		},
		"successRulesetOnlyPostsNothing": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "success",
			Combined:     "combined_status_ruleset_a.yml",
		},
		"pendingDowngradesPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "pending",
			Combined:     "combined_status_mixed.yml",
			Expected:     []string{"policy-bot: main", "policy-bot: ruleset:a", "policy-bot: ruleset:b"},
		},
		"failureRulesetOnlyDowngradesPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "failure",
			Combined:     "combined_status_ruleset_a.yml",
			Expected:     []string{"policy-bot: ruleset:a"},
		},
		"errorDowngradesAllPages": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "error",
			Combined:     "combined_status_paginated.yml",
			Expected:     []string{"policy-bot: ruleset:a", "policy-bot: ruleset:b"},
		},
		"noPrevious": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
			State:        "failure",
			Combined:     "combined_status_ci_only.yml",
		},
		"combinedStatusError": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			State:        "failure",
			Combined:     "server_error.yml",
			Expected:     []string{"policy-bot: main"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rp := &githubtest.ResponsePlayer{}
			statuses := recordStatuses(rp, statusesPath)
			rulesRule := rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+"server_error.yml")
			combinedRule := rp.AddRule(githubtest.ExactPathMatcher(combinedPath), testResponses+test.Combined)

			ec := makeEvalContext(true)
			ec.SkipPostStatus = false
			ec.Client = newTestClient(t, rp)
			ec.Options.StatusChecks = test.StatusChecks
			ec.RulesetChecks = newTestFinder(t)

			ec.PostStatus(context.Background(), test.State, "message")

			assert.Equal(t, test.Expected, statuses.contexts())
			for _, s := range statuses.statuses {
				assert.Equal(t, test.State, s.GetState())
			}

			assert.Equal(t, 4, rulesRule.Count, "rules lookup should be retried 3 times")
			if test.State == "success" {
				assert.Zero(t, combinedRule.Count, "success should not read previous statuses")
			}
		})
	}
}

func TestPostStatusRulesetLookupErrorSkipPostStatus(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	statuses := recordStatuses(rp, statusesPath)
	combinedRule := rp.AddRule(githubtest.ExactPathMatcher("/repos/testorg/testrepo/commits/abc123/status"), testResponses+"combined_status_ruleset_a.yml")

	ec := makeEvalContext(true)
	ec.Client = newTestClient(t, rp)
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckBranch, StatusCheckRuleset}
	ec.RulesetChecks = &fakeRulesetChecks{err: errors.New("lookup failed")}

	ec.PostStatus(context.Background(), "failure", "message")

	assert.Equal(t, []string{"policy-bot: main"}, ec.StatusContexts)
	assert.Empty(t, statuses.statuses)
	assert.Zero(t, combinedRule.Count, "skipped statuses should not read previous statuses")
}

func TestEvaluateWithoutStatusContexts(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	statuses := recordStatuses(rp, statusesPath)

	ec := makeEvalContext(true)
	ec.SkipPostStatus = false
	ec.Client = newTestClient(t, rp)
	ec.Options.StatusChecks = []StatusCheckType{StatusCheckRuleset}
	ec.RulesetChecks = &fakeRulesetChecks{}

	result, err := ec.EvaluatePolicy(context.Background(), fakeEvaluator{status: common.StatusApproved})
	require.NoError(t, err)
	assert.Equal(t, common.StatusApproved, result.Status)
	assert.Equal(t, "evaluated", result.StatusDescription)
	assert.Empty(t, statuses.statuses)
}

func TestExtensionPolicyErrorSources(t *testing.T) {
	const parentPolicy = "policy: {approval: [{slot: {name: service, required: true}}]}"
	for _, test := range []struct {
		name, child, parent, message string
	}{
		{"required slot", extendingPolicy, parentPolicy, "required slots are empty"},
		{"standalone", "policy: {approval: [missing]}", "", "failed to parse approval policy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fetcher := ConfigFetcher{Loader: mockConfigLoader{
				loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(test.child), Source: "example/service@main", Path: "child.yml"}, nil
				},
				loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
					require.NotEmpty(t, test.parent, "only an extending policy may request a parent")
					return appconfig.Config{Content: []byte(test.parent), Source: "shared/policies@release", Path: "rules/parent.yml"}, nil
				},
			}}
			ec := makeEvalContext(false)
			ec.Config = fetcher.ConfigForRepositoryBranch(context.Background(), nil, "example", "service", "main")
			parsed, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			require.ErrorContains(t, err, test.message, "validation failures must reach the handler")
			assert.Nil(t, parsed, "invalid policies must not be evaluated")
			assert.Contains(t, err.Error(), "example/service@main", "diagnostic identifies the child repository")
			assert.Contains(t, err.Error(), "child.yml", "diagnostic identifies the child file")
			require.NotNil(t, ec.Status, "validation failures must post a status")
			assert.Equal(t, "error", ec.Status.GetState(), "invalid policies post an error status")
			assert.Contains(t, ec.Status.GetDescription(), "example/service@main", "status identifies the child repository")
			assert.Contains(t, ec.Status.GetDescription(), "child.yml", "status identifies the child file")
			if test.parent != "" {
				assert.Contains(t, err.Error(), "shared/policies@release", "diagnostic identifies the parent repository")
				assert.Contains(t, err.Error(), "rules/parent.yml", "diagnostic identifies the parent file")
			}
		})
	}
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
			Sources:    []PolicyLocation{{Source: "testorg/testrepo@main", Path: ".policy.yml"}},
			SeenPolicy: seenPolicy,
		},
	}
}
