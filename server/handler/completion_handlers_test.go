// Copyright 2018 Palantir Technologies, Inc.
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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRunEvaluatesFork(t *testing.T) {
	for _, trigger := range []string{"pull_request", "pull_request_target", "push"} {
		t.Run(trigger, func(t *testing.T) {
			lookups, details := 0, 0
			var statuses []github.RepoStatus
			conclusion := "success"
			prState, head := "OPEN", "head"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repos/owner/repo/commits/head/pulls":
					lookups++
					_, _ = w.Write([]byte(`[{"number":42,"state":"open","head":{"sha":"head","repo":{"id":2}},"base":{"repo":{"id":1}}}]`))
				case "/graphql":
					details++
					_, _ = fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"title":"Fork change","state":%q,"createdAt":"2026-01-01T00:00:00Z","author":{"login":"contributor"},"headRefOid":%q,"headRefName":"main","headRepository":{"name":"repo","owner":{"login":"contributor"}},"baseRefName":"main","baseRepository":{"databaseId":1},"isCrossRepository":true,"isDraft":false,"changedFiles":1}}}}`, prState, head)
				case "/repos/owner/repo/commits/head/status":
					_, _ = w.Write([]byte(`{"statuses":[]}`))
				case "/repos/owner/repo/commits/head/check-runs":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"check_runs": []map[string]any{{"name": "ci", "conclusion": conclusion}}}))
				case "/repos/owner/repo/actions/runs":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"workflow_runs": []map[string]any{{"path": ".github/workflows/ci.yml", "event": "pull_request", "status": "completed", "conclusion": conclusion}}}))
				case "/repos/owner/repo/statuses/head":
					var status github.RepoStatus
					require.NoError(t, json.NewDecoder(r.Body).Decode(&status))
					statuses = append(statuses, status)
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(new(server.URL+"/"), nil))
			require.NoError(t, err)
			base := Base{
				ClientCreator:     completionClientCreator{client: client, v4: githubv4.NewEnterpriseClient(server.URL+"/graphql", server.Client())},
				CompletionPRCache: NewCompletionPRCache(),
				BaseConfig:        &baseapp.HTTPConfig{PublicURL: "https://policy.example.com"},
				PullOpts:          &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
				ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
					return appconfig.Config{Content: []byte(`policy:
  approval:
    - CI passes
approval_rules:
  - name: CI passes
    requires:
      conditions:
        has_status:
          statuses: [ci]
        has_workflow_result:
          workflows: [.github/workflows/ci.yml]
`), Source: "owner/repo@main", Path: ".policy.yml"}, nil
				}}},
			}
			payload := map[string]any{
				"action":       "completed",
				"repository":   map[string]any{"id": 1, "name": "repo", "default_branch": "main", "owner": map[string]any{"login": "owner"}},
				"installation": map[string]any{"id": 1},
				"workflow_run": map[string]any{"event": trigger, "head_sha": "head", "head_branch": "main", "conclusion": "success", "pull_requests": []any{}},
			}
			expectedLookups := 1
			if trigger == "push" {
				payload["workflow_run"].(map[string]any)["pull_requests"] = []any{map[string]any{"number": 42, "base": map[string]any{"repo": map[string]any{"id": 1}}}}
				expectedLookups = 0
			}
			data, err := json.Marshal(payload)
			require.NoError(t, err)
			for _, state := range []string{"success", "failure"} {
				conclusion = state
				err = (&WorkflowRun{Base: base}).Handle(context.Background(), "workflow_run", "delivery", data)
				require.NoError(t, err)
			}
			require.Equal(t, expectedLookups, lookups)
			require.Equal(t, 2, details)
			require.Len(t, statuses, 2)
			require.Equal(t, "policy-bot: main", statuses[0].GetContext())
			require.Equal(t, "success", statuses[0].GetState())
			require.Equal(t, "pending", statuses[1].GetState())
			if trigger == "push" {
				return
			}
			for _, current := range []struct{ state, sha string }{{"CLOSED", "head"}, {"OPEN", "new-head"}} {
				prState, head = current.state, current.sha
				err = (&WorkflowRun{Base: base}).Handle(context.Background(), "workflow_run", "delivery", data)
				require.NoError(t, err)
			}
			require.Equal(t, expectedLookups, lookups)
			require.Equal(t, 4, details)
			require.Len(t, statuses, 2)

		})
	}
}

func TestWorkflowRunLookupFailure(t *testing.T) {
	base := Base{ClientCreator: completionClientCreator{err: errors.New("lookup unavailable")}}
	payload := []byte(`{"action":"completed","repository":{"id":1,"name":"repo","owner":{"login":"owner"}},"workflow_run":{"event":"pull_request","head_sha":"head","pull_requests":[]}}`)
	err := (&WorkflowRun{Base: base}).Handle(context.Background(), "workflow_run", "delivery", payload)
	require.ErrorContains(t, err, "lookup unavailable")
}

func TestCompletionHandlersSkipUnassociatedNonPREvents(t *testing.T) {
	for _, kind := range []string{"check_run", "workflow_run"} {
		for _, trigger := range []string{"push", "schedule", "workflow_dispatch", ""} {
			for _, branch := range []string{"main", "feature"} {
				t.Run(kind+"/"+trigger+"/"+branch, func(t *testing.T) {
					// A zero Base panics if the handler attempts any GitHub API access.
					payload := []byte(fmt.Sprintf(`{"action":"completed","repository":{"default_branch":"main"},"%s":{"event":%q,"head_branch":%q,"head_sha":"head","conclusion":"success","pull_requests":[]}}`, kind, trigger, branch))
					var err error
					if kind == "check_run" {
						err = (&CheckRun{}).Handle(context.Background(), kind, "delivery", payload)
					} else {
						err = (&WorkflowRun{}).Handle(context.Background(), kind, "delivery", payload)
					}
					require.NoError(t, err)
				})
			}
		}
	}
}
