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
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAppID = 1234

// fakeGitHub is a minimal GitHub API server for handler tests. Responses
// are keyed by escaped request path and page number. If multiple responses
// are registered for a key, they are returned in order and the last one is
// repeated.
type fakeGitHub struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	responses map[string][]fakeResponse
	failures  map[string]int
	requests  []string
	headers   []http.Header
	statuses  []github.RepoStatus
}

type fakeResponse struct {
	status   int
	body     any
	nextPage int
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{t: t, responses: make(map[string][]fakeResponse), failures: make(map[string]int)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) Client() *github.Client {
	base := f.srv.URL + "/"
	client, err := github.NewClient(github.WithURLs(&base, &base))
	require.NoError(f.t, err)
	return client
}

// Respond registers a response for an escaped path and page, where page 0
// matches requests without a page parameter.
func (f *fakeGitHub) Respond(path string, page int, res fakeResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fmt.Sprintf("%s#%d", path, page)
	f.responses[key] = append(f.responses[key], res)
}

// Fail responds with a server error to the next n requests for an escaped path.
func (f *fakeGitHub) Fail(path string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[path] = n
}

func (f *fakeGitHub) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeGitHub) Headers() []http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]http.Header(nil), f.headers...)
}

func (f *fakeGitHub) Statuses() []github.RepoStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]github.RepoStatus(nil), f.statuses...)
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.EscapedPath()
	f.requests = append(f.requests, r.Method+" "+path)
	f.headers = append(f.headers, r.Header.Clone())

	if r.Method == http.MethodPost {
		var status github.RepoStatus
		if err := json.NewDecoder(r.Body).Decode(&status); err == nil {
			f.statuses = append(f.statuses, status)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
		return
	}

	if f.failures[path] > 0 {
		f.failures[path]--
		http.Error(w, `{"message": "Server Error"}`, http.StatusInternalServerError)
		return
	}

	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		_, _ = fmt.Sscanf(p, "%d", &page)
	}

	key := fmt.Sprintf("%s#%d", path, page)
	if _, ok := f.responses[key]; !ok && page == 1 {
		key = fmt.Sprintf("%s#%d", path, 0)
	}
	queued := f.responses[key]
	if len(queued) == 0 {
		http.Error(w, `{"message": "Not Found"}`, http.StatusNotFound)
		return
	}
	res := queued[0]
	if len(queued) > 1 {
		f.responses[key] = queued[1:]
	}

	if res.nextPage > 0 {
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d>; rel="next"`, f.srv.URL, path, res.nextPage))
	}
	w.Header().Set("Content-Type", "application/json")
	if res.status != 0 {
		w.WriteHeader(res.status)
	}
	_ = json.NewEncoder(w).Encode(res.body)
}

type testCheck struct {
	context       string
	integrationID int64
}

func statusChecksRule(rulesetID int64, checks ...testCheck) map[string]any {
	var required []map[string]any
	for _, c := range checks {
		check := map[string]any{"context": c.context}
		if c.integrationID != 0 {
			check["integration_id"] = c.integrationID
		}
		required = append(required, check)
	}
	return map[string]any{
		"type":                "required_status_checks",
		"ruleset_source_type": "Repository",
		"ruleset_source":      "testorg/testrepo",
		"ruleset_id":          rulesetID,
		"parameters": map[string]any{
			"required_status_checks":               required,
			"strict_required_status_checks_policy": false,
		},
	}
}

func rulesets(names map[int64]string) []map[string]any {
	var rs []map[string]any
	for id, name := range names {
		rs = append(rs, map[string]any{"id": id, "name": name, "source": "testorg/testrepo", "enforcement": "active"})
	}
	return rs
}

func newTestFinder(t *testing.T) *RulesetCheckFinder {
	f := NewRulesetCheckFinder(testAppID, "policy-bot")
	f.retryDelay = time.Millisecond
	return f
}

func TestRulesetCheckFinder(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	names := map[int64]string{1: "release-branches", 2: "other"}

	tests := map[string]struct {
		Rules    []map[string]any
		Expected []string
		Ignored  map[string]int64
	}{
		"pinnedMatchingName": {
			Rules: []map[string]any{
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Expected: []string{"policy-bot: ruleset:release-branches"},
		},
		"unpinned": {
			Rules: []map[string]any{
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", 0}),
			},
			Ignored: map[string]int64{ignoreReasonUnpinned: 1},
		},
		"foreignIntegration": {
			Rules: []map[string]any{
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", 9999}),
			},
			Ignored: map[string]int64{ignoreReasonWrongIntegration: 1},
		},
		"nameMismatch": {
			Rules: []map[string]any{
				statusChecksRule(2, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Ignored: map[string]int64{ignoreReasonNameMismatch: 1},
		},
		"nonPolicyBotChecks": {
			Rules: []map[string]any{
				statusChecksRule(1,
					testCheck{"ci/build", testAppID},
					testCheck{"policy-bot: main", testAppID},
					testCheck{"other-bot: ruleset:release-branches", testAppID},
				),
			},
		},
		"multipleRulesetsDeduplicated": {
			Rules: []map[string]any{
				statusChecksRule(2, testCheck{"policy-bot: ruleset:other", testAppID}),
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Expected: []string{"policy-bot: ruleset:other", "policy-bot: ruleset:release-branches"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)
			gh.Respond(rulesPath, 0, fakeResponse{body: test.Rules})
			gh.Respond(rulesetsPath, 0, fakeResponse{body: rulesets(names)})

			registry := metrics.NewRegistry()
			ctx := baseapp.WithMetricsCtx(context.Background(), registry)

			checks, err := newTestFinder(t).RequiredChecks(ctx, gh.Client(), "testorg", "testrepo", "main")
			require.NoError(t, err)
			assert.Equal(t, test.Expected, checks)

			for _, reason := range []string{ignoreReasonUnpinned, ignoreReasonWrongIntegration, ignoreReasonNameMismatch, ignoreReasonUnknownRuleset, ignoreReasonShadowed} {
				counter := metrics.GetOrRegisterCounter(fmt.Sprintf("%s[reason:%s]", MetricsKeyRulesetChecksIgnored, reason), registry)
				assert.Equal(t, test.Ignored[reason], counter.Count(), "incorrect ignored count for %s", reason)
			}
		})
	}
}

type testRuleset struct {
	id         int64
	name       string
	sourceType string
}

func scopedRulesets(rulesets ...testRuleset) []map[string]any {
	var rs []map[string]any
	for _, r := range rulesets {
		rs = append(rs, map[string]any{"id": r.id, "name": r.name, "source_type": r.sourceType, "source": "testorg", "enforcement": "active"})
	}
	return rs
}

func TestRulesetCheckFinderScopePrecedence(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	repo := func(id int64, name string) testRuleset { return testRuleset{id, name, "Repository"} }
	org := func(id int64, name string) testRuleset { return testRuleset{id, name, "Organization"} }
	enterprise := func(id int64, name string) testRuleset { return testRuleset{id, name, "Enterprise"} }

	tests := map[string]struct {
		Rulesets []testRuleset
		Rules    []map[string]any
		Expected []string
		Shadowed int64
	}{
		"repoShadowedByOrg": {
			Rulesets: []testRuleset{org(1, "release-branches"), repo(2, "release-branches")},
			Rules: []map[string]any{
				statusChecksRule(2, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Shadowed: 1,
		},
		"orgPostedWhenRepoUsesSameName": {
			Rulesets: []testRuleset{org(1, "release-branches"), repo(2, "release-branches")},
			Rules: []map[string]any{
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Expected: []string{"policy-bot: ruleset:release-branches"},
		},
		"orgShadowedByEnterprise": {
			Rulesets: []testRuleset{enterprise(1, "release-branches"), org(2, "release-branches")},
			Rules: []map[string]any{
				statusChecksRule(2, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
			},
			Shadowed: 1,
		},
		"distinctNamesAcrossScopes": {
			Rulesets: []testRuleset{org(1, "release-branches"), repo(2, "hotfix-branches")},
			Rules: []map[string]any{
				statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
				statusChecksRule(2, testCheck{"policy-bot: ruleset:hotfix-branches", testAppID}),
			},
			Expected: []string{"policy-bot: ruleset:hotfix-branches", "policy-bot: ruleset:release-branches"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)
			gh.Respond(rulesPath, 0, fakeResponse{body: test.Rules})
			gh.Respond(rulesetsPath, 0, fakeResponse{body: scopedRulesets(test.Rulesets...)})

			registry := metrics.NewRegistry()
			ctx := baseapp.WithMetricsCtx(context.Background(), registry)

			checks, err := newTestFinder(t).RequiredChecks(ctx, gh.Client(), "testorg", "testrepo", "main")
			require.NoError(t, err)
			assert.Equal(t, test.Expected, checks)

			counter := metrics.GetOrRegisterCounter(fmt.Sprintf("%s[reason:%s]", MetricsKeyRulesetChecksIgnored, ignoreReasonShadowed), registry)
			assert.Equal(t, test.Shadowed, counter.Count())
		})
	}
}

func TestRulesetCheckFinderPagination(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	gh := newFakeGitHub(t)
	gh.Respond(rulesPath, 0, fakeResponse{nextPage: 2, body: []map[string]any{
		statusChecksRule(1, testCheck{"policy-bot: ruleset:first", testAppID}),
	}})
	gh.Respond(rulesPath, 2, fakeResponse{body: []map[string]any{
		statusChecksRule(2, testCheck{"policy-bot: ruleset:second", testAppID}),
	}})
	gh.Respond(rulesetsPath, 0, fakeResponse{nextPage: 2, body: rulesets(map[int64]string{1: "first"})})
	gh.Respond(rulesetsPath, 2, fakeResponse{body: rulesets(map[int64]string{2: "second"})})

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:first", "policy-bot: ruleset:second"}, checks)
}

func TestRulesetCheckFinderEscapesBranch(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.Respond("/repos/testorg/testrepo/rules/branches/release%2F1.0", 0, fakeResponse{body: []map[string]any{
		statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
	}})
	gh.Respond("/repos/testorg/testrepo/rulesets", 0, fakeResponse{body: rulesets(map[int64]string{1: "release-branches"})})

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "release/1.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:release-branches"}, checks)
	assert.Contains(t, gh.Requests(), "GET /repos/testorg/testrepo/rules/branches/release%2F1.0")
}

func TestRulesetCheckFinderRevalidatesUnknownRuleset(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	gh := newFakeGitHub(t)
	gh.Respond(rulesPath, 0, fakeResponse{body: []map[string]any{
		statusChecksRule(1, testCheck{"policy-bot: ruleset:new", testAppID}),
	}})
	// the first response simulates a stale cached response without the new ruleset
	gh.Respond(rulesetsPath, 0, fakeResponse{body: rulesets(map[int64]string{2: "old"})})
	gh.Respond(rulesetsPath, 0, fakeResponse{body: rulesets(map[int64]string{1: "new", 2: "old"})})

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:new"}, checks)

	requests, headers := gh.Requests(), gh.Headers()
	require.Equal(t, []string{"GET " + rulesPath, "GET " + rulesetsPath, "GET " + rulesetsPath}, requests)
	assert.Empty(t, headers[1].Get("Cache-Control"))
	assert.Equal(t, "max-age=0", headers[2].Get("Cache-Control"))
}

func TestRulesetCheckFinderRetriesServerErrors(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	gh := newFakeGitHub(t)
	gh.Respond(rulesPath, 0, fakeResponse{body: []map[string]any{
		statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
	}})
	gh.Respond(rulesetsPath, 0, fakeResponse{body: rulesets(map[int64]string{1: "release-branches"})})
	gh.Fail(rulesPath, 1)
	gh.Fail(rulesetsPath, 1)

	checks, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"policy-bot: ruleset:release-branches"}, checks)
	assert.Len(t, gh.Requests(), 4)

	t.Run("onlyOnce", func(t *testing.T) {
		gh := newFakeGitHub(t)
		gh.Respond(rulesPath, 0, fakeResponse{body: []map[string]any{}})
		gh.Fail(rulesPath, 2)

		_, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
		require.Error(t, err)
		assert.Len(t, gh.Requests(), 2)
	})
}

func TestRulesetCheckFinderDoesNotRetryClientErrors(t *testing.T) {
	gh := newFakeGitHub(t)

	_, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
	require.Error(t, err)
	assert.Len(t, gh.Requests(), 1, "404 responses should not be retried")
}

func TestRulesetCheckFinderErrors(t *testing.T) {
	const rulesPath = "/repos/testorg/testrepo/rules/branches/main"
	const rulesetsPath = "/repos/testorg/testrepo/rulesets"

	t.Run("rules", func(t *testing.T) {
		gh := newFakeGitHub(t)
		gh.Respond(rulesPath, 0, fakeResponse{status: http.StatusInternalServerError, body: map[string]any{}})

		registry := metrics.NewRegistry()
		ctx := baseapp.WithMetricsCtx(context.Background(), registry)

		checks, err := newTestFinder(t).RequiredChecks(ctx, gh.Client(), "testorg", "testrepo", "main")
		require.Error(t, err)
		assert.Nil(t, checks)
		assert.Equal(t, int64(1), metrics.GetOrRegisterCounter(MetricsKeyRulesetLookupErrors, registry).Count())
	})

	t.Run("names", func(t *testing.T) {
		gh := newFakeGitHub(t)
		gh.Respond(rulesPath, 0, fakeResponse{body: []map[string]any{
			statusChecksRule(1, testCheck{"policy-bot: ruleset:release-branches", testAppID}),
		}})
		gh.Respond(rulesetsPath, 0, fakeResponse{status: http.StatusInternalServerError, body: map[string]any{}})

		checks, err := newTestFinder(t).RequiredChecks(context.Background(), gh.Client(), "testorg", "testrepo", "main")
		require.Error(t, err)
		assert.Nil(t, checks)
	})
}
