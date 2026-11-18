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
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueCommentTamperingUsesDefaultMethods(t *testing.T) {
	tests := map[string]string{
		"rule methods": `
policy:
  approval:
    - one approval

approval_rules:
  - name: one approval
    options:
      methods:
        comments: ["LGTM"]
    requires:
      count: 1
      organizations: ["testorg"]
`,
		"hardcoded defaults": `
policy:
  approval:
    - one approval

approval_rules:
  - name: one approval
    requires:
      count: 1
      organizations: ["testorg"]
`,
		"policy defaults": `
policy:
  approval:
    - one approval

approval_defaults:
  options:
    methods:
      comments: ["LGTM"]

approval_rules:
  - name: one approval
    requires:
      count: 1
      organizations: ["testorg"]
`,
	}

	// Another user edits an unrelated comment so that it matches the approval
	// methods of every policy in the test
	event := github.IssueCommentEvent{
		Action: new("edited"),
		Issue: &github.Issue{
			Number:           new(42),
			PullRequestLinks: &github.PullRequestLinks{},
		},
		Comment: &github.IssueComment{
			Body: new(":+1: LGTM"),
			User: &github.User{Login: new("alice")},
		},
		Changes: &github.EditChange{
			Body: &github.EditBody{From: new("needs more tests")},
		},
		Repo: &github.Repository{
			Name:  new("testrepo"),
			Owner: &github.User{Login: new("testorg")},
		},
		Sender:       &github.User{Login: new("mallory")},
		Installation: &github.Installation{ID: new(int64(1))},
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)

			h := IssueComment{
				Base: Base{
					ClientCreator: gh,
					ConfigFetcher: &ConfigFetcher{
						Loader: mockConfigLoader{
							loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
								return appconfig.Config{
									Content: []byte(content),
									Source:  "testorg/testrepo@main",
									Path:    ".policy.yml",
								}, nil
							},
						},
					},
					BaseConfig: &baseapp.HTTPConfig{PublicURL: "https://policy-bot.example.com"},
					PullOpts:   &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
					AppName:    "policy-bot",
				},
			}

			require.NoError(t, h.Handle(context.Background(), "issue_comment", "1", payload))

			statuses := gh.Statuses()
			require.Len(t, statuses, 1, "incorrect number of statuses")
			assert.Equal(t, "policy-bot: main", statuses[0].GetContext())
			assert.Equal(t, "failure", statuses[0].GetState())
			assert.Equal(t, "Entity mallory edited approval comment by alice", statuses[0].GetDescription())
		})
	}
}

// fakeGitHub is a ClientCreator that returns clients for a test server. The
// server responds with a fixed pull request and records all posted statuses.
type fakeGitHub struct {
	githubapp.ClientCreator

	server *httptest.Server

	mu       sync.Mutex
	statuses []*github.RepoStatus
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	repo := &github.Repository{
		ID:    new(int64(1234)),
		Name:  new("testrepo"),
		Owner: &github.User{Login: new("testorg")},
	}

	pr := &github.PullRequest{
		Number:       new(42),
		Title:        new("Test pull request"),
		State:        new("open"),
		Draft:        new(false),
		CreatedAt:    &github.Timestamp{Time: time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)},
		User:         &github.User{Login: new("bob")},
		ChangedFiles: new(1),
		Base:         &github.PullRequestBranch{Ref: new("main"), Repo: repo},
		Head:         &github.PullRequestBranch{Ref: new("feature"), SHA: new("abc123"), Repo: repo},
	}

	gh := &fakeGitHub{}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/testorg/testrepo/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(pr)
	})
	mux.HandleFunc("POST /repos/testorg/testrepo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
		var status github.RepoStatus
		if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		gh.mu.Lock()
		gh.statuses = append(gh.statuses, &status)
		gh.mu.Unlock()

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(&status)
	})

	gh.server = httptest.NewServer(mux)
	t.Cleanup(gh.server.Close)

	return gh
}

func (gh *fakeGitHub) Statuses() []*github.RepoStatus {
	gh.mu.Lock()
	defer gh.mu.Unlock()
	return slices.Clone(gh.statuses)
}

func (gh *fakeGitHub) NewInstallationClient(installationID int64) (*github.Client, error) {
	return github.NewClient(github.WithURLs(new(gh.server.URL+"/"), nil))
}

func (gh *fakeGitHub) NewInstallationV4Client(installationID int64) (*githubv4.Client, error) {
	return githubv4.NewEnterpriseClient(gh.server.URL+"/graphql", nil), nil
}
