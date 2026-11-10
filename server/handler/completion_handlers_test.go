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

func TestCompletionHandlersEvaluateFork(t *testing.T) {
	for _, kind := range []string{"check_run", "workflow_run"} {
		t.Run(kind, func(t *testing.T) {
			lookups, details := 0, 0
			var statuses []github.RepoStatus
			conclusion := "success"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repos/owner/repo/commits/head/pulls":
					lookups++
					_, _ = w.Write([]byte(`[{"number":42,"state":"open","head":{"sha":"head","repo":{"id":2}},"base":{"repo":{"id":1}}}]`))
				case "/graphql":
					details++
					_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"title":"Fork change","state":"OPEN","createdAt":"2026-01-01T00:00:00Z","author":{"login":"contributor"},"headRefOid":"head","headRefName":"main","headRepository":{"name":"repo","owner":{"login":"contributor"}},"baseRefName":"main","baseRepository":{"databaseId":1},"isCrossRepository":true,"isDraft":false,"changedFiles":1}}}}`))
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
				kind:           map[string]any{"head_sha": "head", "head_branch": "main", "conclusion": "success", "pull_requests": []any{}},
			}
			data, err := json.Marshal(payload)
			require.NoError(t, err)
			for _, state := range []string{"success", "failure"} {
				conclusion = state
				if kind == "check_run" {
					err = (&CheckRun{Base: base}).Handle(context.Background(), kind, "delivery", data)
				} else {
					err = (&WorkflowRun{Base: base}).Handle(context.Background(), kind, "delivery", data)
				}
				require.NoError(t, err)
			}
			require.Equal(t, 1, lookups)
			require.Equal(t, 2, details)
			require.Len(t, statuses, 2)
			require.Equal(t, "policy-bot: main", statuses[0].GetContext())
			require.Equal(t, "success", statuses[0].GetState())
			require.Equal(t, "pending", statuses[1].GetState())
		})
	}
}

func TestCompletionHandlersLookupFailure(t *testing.T) {
	for _, kind := range []string{"check_run", "workflow_run"} {
		t.Run(kind, func(t *testing.T) {
			base := Base{ClientCreator: completionClientCreator{err: errors.New("lookup unavailable")}}
			payload := []byte(fmt.Sprintf(`{"action":"completed","repository":{"id":1,"name":"repo","owner":{"login":"owner"}},"%s":{"head_sha":"head","conclusion":"success","pull_requests":[]}}`, kind))
			var err error
			if kind == "check_run" {
				err = (&CheckRun{Base: base}).Handle(context.Background(), kind, "delivery", payload)
			} else {
				err = (&WorkflowRun{Base: base}).Handle(context.Background(), kind, "delivery", payload)
			}
			require.ErrorContains(t, err, "lookup unavailable")
		})
	}
}
