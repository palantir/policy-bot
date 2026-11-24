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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/policy-bot/internal/githubtest"
	"github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAppID     = 1234
	testResponses = "testdata/responses/"

	rulesPath    = "/repos/testorg/testrepo/rules/branches/main"
	rulesetsPath = "/repos/testorg/testrepo/rulesets"
	statusesPath = "/repos/testorg/testrepo/statuses/abc123"
)

func newTestClient(t *testing.T, rp *githubtest.ResponsePlayer) *github.Client {
	baseURL := "http://github.localhost/"
	client, err := github.NewClient(
		github.WithHTTPClient(&http.Client{Transport: rp}),
		github.WithURLs(&baseURL, nil),
	)
	require.NoError(t, err, "failed to create github client")
	return client
}

// statusRecorder matches requests that create commit statuses and records
// the statuses they create.
type statusRecorder struct {
	path     string
	statuses []github.RepoStatus
}

func (r *statusRecorder) Matches(req *http.Request, body []byte) bool {
	if req.Method != http.MethodPost || req.URL.Path != r.path {
		return false
	}
	var status github.RepoStatus
	if err := json.Unmarshal(body, &status); err == nil {
		r.statuses = append(r.statuses, status)
	}
	return true
}

func (r *statusRecorder) contexts() []string {
	var contexts []string
	for _, s := range r.statuses {
		contexts = append(contexts, s.GetContext())
	}
	return contexts
}

func recordStatuses(rp *githubtest.ResponsePlayer, path string) *statusRecorder {
	recorder := &statusRecorder{path: path}
	rp.AddRule(recorder, testResponses+"status_created.yml")
	return recorder
}

func newTestFinder(t *testing.T) *RulesetCheckFinder {
	setTestRetryDelay(t)
	return NewRulesetCheckFinder(testAppID, "policy-bot")
}

// setTestRetryDelay shortens retries of GitHub requests for the test.
func setTestRetryDelay(t *testing.T) {
	delay := retryDelay
	retryDelay = time.Millisecond
	t.Cleanup(func() { retryDelay = delay })
}

func ignoredCount(registry metrics.Registry, reason string) int64 {
	return metrics.GetOrRegisterCounter(fmt.Sprintf("%s[reason:%s]", MetricsKeyRulesetChecksIgnored, reason), registry).Count()
}

func TestRulesetCheckFinder(t *testing.T) {
	tests := map[string]struct {
		Rules    string
		Rulesets string
		Expected []string
		Ignored  map[string]int64
	}{
		"pinnedMatchingName": {
			Rules:    "rules_id1_release_branches.yml",
			Expected: []string{"policy-bot: ruleset:release-branches"},
		},
		"unpinned": {
			Rules:   "rules_id1_unpinned.yml",
			Ignored: map[string]int64{ignoreReasonUnpinned: 1},
		},
		"foreignIntegration": {
			Rules:   "rules_id1_foreign_app.yml",
			Ignored: map[string]int64{ignoreReasonWrongIntegration: 1},
		},
		"nameMismatch": {
			Rules:   "rules_id2_release_branches.yml",
			Ignored: map[string]int64{ignoreReasonNameMismatch: 1},
		},
		"multipleRulesetsDeduplicated": {
			Rules:    "rules_multiple_rulesets.yml",
			Expected: []string{"policy-bot: ruleset:other", "policy-bot: ruleset:release-branches"},
		},
		"unknownRuleset": {
			Rules:   "rules_id3_hidden.yml",
			Ignored: map[string]int64{ignoreReasonUnknownRuleset: 1},
		},
		"repoShadowedByOrg": {
			Rules:    "rules_id2_release_branches.yml",
			Rulesets: "rulesets_org_repo_same_name.yml",
			Ignored:  map[string]int64{ignoreReasonShadowed: 1},
		},
		"orgPostedWhenRepoUsesSameName": {
			Rules:    "rules_id1_release_branches.yml",
			Rulesets: "rulesets_org_repo_same_name.yml",
			Expected: []string{"policy-bot: ruleset:release-branches"},
		},
		"orgShadowedByEnterprise": {
			Rules:    "rules_id2_release_branches.yml",
			Rulesets: "rulesets_enterprise_org_same_name.yml",
			Ignored:  map[string]int64{ignoreReasonShadowed: 1},
		},
		"distinctNamesAcrossScopes": {
			Rules:    "rules_distinct_scopes.yml",
			Rulesets: "rulesets_distinct_scopes.yml",
			Expected: []string{"policy-bot: ruleset:hotfix-branches", "policy-bot: ruleset:release-branches"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.Rulesets == "" {
				test.Rulesets = "rulesets.yml"
			}

			rp := &githubtest.ResponsePlayer{}
			rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+test.Rules)
			rulesetsRule := rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+test.Rulesets)

			registry := metrics.NewRegistry()
			ctx := baseapp.WithMetricsCtx(context.Background(), registry)

			checks, err := newTestFinder(t).RequiredChecks(ctx, newTestClient(t, rp), "testorg", "testrepo", "main")
			require.NoError(t, err)
			assert.Equal(t, test.Expected, checks)
			assert.Equal(t, 1, rulesetsRule.Count, "rulesets should be fetched once")

			for _, reason := range []string{ignoreReasonUnpinned, ignoreReasonWrongIntegration, ignoreReasonNameMismatch, ignoreReasonUnknownRuleset, ignoreReasonShadowed} {
				assert.Equal(t, test.Ignored[reason], ignoredCount(registry, reason), "incorrect ignored count for %s", reason)
			}
		})
	}
}

func TestRulesetCheckFinderSkipsRulesetsWithoutPolicyBotChecks(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+"rules_non_policy_bot.yml")
	rulesetsRule := rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+"rulesets.yml")

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), newTestClient(t, rp), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Empty(t, checks)
	assert.Zero(t, rulesetsRule.Count, "rulesets should not be fetched without policy-bot checks")
}

func TestRulesetCheckFinderPagination(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	rulesRule := rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+"rules_paginated.yml")
	rulesetsRule := rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+"rulesets_paginated.yml")

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), newTestClient(t, rp), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:first", "policy-bot: ruleset:second"}, checks)
	assert.Equal(t, 2, rulesRule.Count)
	assert.Equal(t, 2, rulesetsRule.Count)
}

func TestRulesetCheckFinderEscapesBranch(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	rulesRule := rp.AddRule(
		githubtest.EscapedPathMatcher("/repos/testorg/testrepo/rules/branches/release%2F1.0"),
		testResponses+"rules_id1_release_branches.yml",
	)
	rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+"rulesets.yml")

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), newTestClient(t, rp), "testorg", "testrepo", "release/1.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:release-branches"}, checks)
	assert.Equal(t, 1, rulesRule.Count)
}

func TestRulesetCheckFinderRetriesServerErrors(t *testing.T) {
	rp := &githubtest.ResponsePlayer{}
	rulesRule := rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+"rules_retry.yml")
	rulesetsRule := rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+"rulesets_retry.yml")

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), newTestClient(t, rp), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:release-branches"}, checks)
	assert.Equal(t, 2, rulesRule.Count)
	assert.Equal(t, 2, rulesetsRule.Count)
}

func TestRulesetCheckFinderErrors(t *testing.T) {
	tests := map[string]struct {
		Rules         string
		Rulesets      string
		RulesRequests int
	}{
		"rulesServerError": {
			Rules:         "server_error.yml",
			RulesRequests: 4,
		},
		"rulesNotFound": {
			Rules:         "not_found.yml",
			RulesRequests: 1,
		},
		"rulesetsServerError": {
			Rules:         "rules_id1_release_branches.yml",
			Rulesets:      "server_error.yml",
			RulesRequests: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rp := &githubtest.ResponsePlayer{}
			rulesRule := rp.AddRule(githubtest.ExactPathMatcher(rulesPath), testResponses+test.Rules)
			if test.Rulesets != "" {
				rp.AddRule(githubtest.ExactPathMatcher(rulesetsPath), testResponses+test.Rulesets)
			}

			registry := metrics.NewRegistry()
			ctx := baseapp.WithMetricsCtx(context.Background(), registry)

			checks, err := newTestFinder(t).RequiredChecks(ctx, newTestClient(t, rp), "testorg", "testrepo", "main")
			require.Error(t, err)
			assert.Nil(t, checks)
			assert.Equal(t, test.RulesRequests, rulesRule.Count)
			assert.Equal(t, int64(1), metrics.GetOrRegisterCounter(MetricsKeyRulesetLookupErrors, registry).Count())
		})
	}
}
