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
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/internal/responsetest"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRunEvaluatesFork(t *testing.T) {
	tests := map[string]struct {
		trigger             string
		withAssociation     bool
		pullRequestResponse string
		conclusion          string
		expectedStatus      string
	}{
		"pull_request fork":         {trigger: "pull_request", pullRequestResponse: "workflow_pull_request_open.yml", conclusion: "success", expectedStatus: "success"},
		"pull_request_target fork":  {trigger: "pull_request_target", pullRequestResponse: "workflow_pull_request_open.yml", conclusion: "success", expectedStatus: "success"},
		"failed workflow":           {trigger: "pull_request", pullRequestResponse: "workflow_pull_request_open.yml", conclusion: "failure", expectedStatus: "pending"},
		"closed after lookup":       {trigger: "pull_request", pullRequestResponse: "workflow_pull_request_closed.yml", conclusion: "success"},
		"head changed after lookup": {trigger: "pull_request", pullRequestResponse: "workflow_pull_request_moved.yml", conclusion: "success"},
		"push with association":     {trigger: "push", withAssociation: true, pullRequestResponse: "workflow_pull_request_open.yml", conclusion: "success", expectedStatus: "success"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rp := &responsetest.ResponsePlayer{}
			lookup := rp.AddRule(
				responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"),
				"testdata/responses/completion_pull_requests.yml",
			)
			// The lookup still reports an open PR at head; GraphQL supplies its newer state.
			details := rp.AddRule(
				responsetest.ExactPathMatcher("/graphql"),
				"testdata/responses/"+tt.pullRequestResponse,
			)
			rp.AddRule(
				responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/status"),
				"testdata/responses/completion_statuses.yml",
			)
			rp.AddRule(
				responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/check-runs"),
				"testdata/responses/completion_checks_"+tt.conclusion+".yml",
			)
			rp.AddRule(
				responsetest.ExactPathMatcher("/repos/owner/repo/actions/runs"),
				"testdata/responses/completion_workflows_"+tt.conclusion+".yml",
			)
			var statuses []github.RepoStatus
			rp.AddRule(
				statusRequestMatcher{t: t, statuses: &statuses},
				"testdata/responses/completion_post_status.yml",
			)
			httpClient := &http.Client{Transport: rp}
			client, err := github.NewClient(github.WithHTTPClient(httpClient))
			require.NoError(t, err)
			base := Base{
				ClientCreator: mockCompletionClientCreator{client: client, v4: githubv4.NewEnterpriseClient("https://api.github.com/graphql", httpClient)},
				BaseConfig:    &baseapp.HTTPConfig{PublicURL: "https://policy.example.com"},
				PullOpts:      &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
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
			payload := github.WorkflowRunEvent{
				Action: new("completed"),
				Repo: &github.Repository{
					ID:            new(int64(1)),
					Name:          new("repo"),
					DefaultBranch: new("main"),
					Owner:         &github.User{Login: new("owner")},
				},
				Installation: &github.Installation{ID: new(int64(1))},
				WorkflowRun: &github.WorkflowRun{
					Event:      new(tt.trigger),
					HeadSHA:    new("head"),
					HeadBranch: new("main"),
					Conclusion: new(tt.conclusion),
				},
			}
			expectedLookups := 1
			if tt.withAssociation {
				payload.WorkflowRun.PullRequests = []*github.PullRequest{{
					Number: new(42),
					Base:   &github.PullRequestBranch{Repo: payload.Repo},
				}}
				expectedLookups = 0
			}
			data, err := json.Marshal(payload)
			require.NoError(t, err)
			require.NoError(t, (&WorkflowRun{Base: base}).Handle(context.Background(), "workflow_run", "delivery", data))
			assert.Equal(t, expectedLookups, lookup.Count, "association lookup count")
			assert.Equal(t, 1, details.Count, "current PR state must be loaded")
			if tt.expectedStatus == "" {
				assert.Empty(t, statuses, "closed or moved PR must not receive a status")
			} else if assert.Len(t, statuses, 1, "one policy status should be posted") {
				assert.Equal(t, "policy-bot: main", statuses[0].GetContext(), "policy status context")
				assert.Equal(t, tt.expectedStatus, statuses[0].GetState(), "policy result")
			}
		})
	}
}

type statusRequestMatcher struct {
	t        *testing.T
	statuses *[]github.RepoStatus
}

func (m statusRequestMatcher) Matches(r *http.Request, body []byte) bool {
	if r.Method != http.MethodPost || r.URL.Path != "/repos/owner/repo/statuses/head" {
		return false
	}
	var status github.RepoStatus
	require.NoError(m.t, json.Unmarshal(body, &status))
	*m.statuses = append(*m.statuses, status)
	return true
}

func TestWorkflowRunLookupFailure(t *testing.T) {
	base := Base{ClientCreator: mockCompletionClientCreator{err: errors.New("lookup unavailable")}}
	payload := []byte(`{"action":"completed","repository":{"id":1,"name":"repo","owner":{"login":"owner"}},"workflow_run":{"event":"pull_request","head_sha":"head","pull_requests":[]}}`)
	err := (&WorkflowRun{Base: base}).Handle(context.Background(), "workflow_run", "delivery", payload)
	require.ErrorContains(t, err, "lookup unavailable")
}

func TestWorkflowRunSkipsUnassociatedNonPREvents(t *testing.T) {
	for _, trigger := range []string{"push", "schedule", "workflow_dispatch", "workflow_run", ""} {
		for _, branch := range []string{"main", "feature"} {
			t.Run(trigger+"/"+branch, func(t *testing.T) {
				// A zero Base panics if the handler attempts any GitHub API access.
				payload := []byte(fmt.Sprintf(`{"action":"completed","repository":{"default_branch":"main"},"workflow_run":{"event":%q,"head_branch":%q,"head_sha":"head","conclusion":"success","pull_requests":[]}}`, trigger, branch))
				require.NoError(t, (&WorkflowRun{}).Handle(context.Background(), "workflow_run", "delivery", payload))
			})
		}
	}
}
