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
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestReviewCommentOnlyMethods(t *testing.T) {
	const standalone = `
policy:
  approval: [Comment approval]
approval_defaults:
  options:
    methods: {comments: [ship], github_review: false}
approval_rules:
  - name: Comment approval
    requires: {count: 1, users: [reviewer]}
`
	slotted := strings.Replace(standalone, "approval: [Comment approval]", "approval: [{slot: service}]", 1) + "\nslots: {service: [Comment approval]}"
	repo := &github.Repository{ID: new(int64(1)), Name: new("service"), Owner: &github.User{Login: new("example")}}
	pr := &github.PullRequest{
		Title: new("Change"), State: new("open"), Number: new(1), Draft: new(false),
		CreatedAt: &github.Timestamp{Time: time.Unix(1, 0)}, User: &github.User{Login: new("author")},
		Base:         &github.PullRequestBranch{Ref: new("main"), Repo: repo},
		Head:         &github.PullRequestBranch{Ref: new("feature"), SHA: new("head"), Repo: repo},
		ChangedFiles: new(1),
	}
	for _, source := range []struct {
		name, child, parent string
	}{
		{"standalone", standalone, ""},
		{"standalone slot", slotted, ""},
		{"parent slot default", extensionPolicy, slotted},
	} {
		t.Run(source.name, func(t *testing.T) {
			for _, test := range []struct {
				name, action, state, body, previous, sender, status string
			}{
				{"matching comment", "submitted", "commented", "ship", "", "reviewer", "success"},
				{"unrelated comment", "submitted", "commented", "unrelated", "", "reviewer", ""},
				{"edited approval removed", "edited", "commented", "removed", "ship", "reviewer", "pending"},
				{"dismissed comment", "dismissed", "dismissed", "ship", "", "maintainer", "pending"},
				{"own dismissal", "dismissed", "dismissed", "ship", "", "policy-bot[bot]", ""},
			} {
				t.Run(test.name, func(t *testing.T) {
					statuses := make(chan *github.RepoStatus, 2)
					queries := make(chan struct{}, 2)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.Method + " " + r.URL.Path {
						case "POST /graphql":
							queries <- struct{}{}
							// Dismissed reviews are excluded by the client's GraphQL query.
							reviews := "[]"
							if test.state == "commented" {
								reviews = fmt.Sprintf(`[{"id":"review","author":{"login":"reviewer"},"state":"COMMENTED","body":%q,"submittedAt":"2026-01-01T00:00:00Z","commit":{"oid":"head"}}]`, test.body)
							}
							_, err := fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{
"commits":{"nodes":[{"commit":{"oid":"head","author":{"user":{"login":"author"}},"parents":{"nodes":[{"oid":"base"}]}}}]},
"comments":{"nodes":[]},"reviews":{"nodes":%s}}}}}`, reviews)
							assert.NoError(t, err)
						case "POST /repos/example/service/statuses/head":
							status := new(github.RepoStatus)
							if !assert.NoError(t, json.NewDecoder(r.Body).Decode(status)) {
								return
							}
							statuses <- status
							assert.NoError(t, json.NewEncoder(w).Encode(status))
						default:
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							http.Error(w, "unexpected request", http.StatusBadRequest)
						}
					}))
					defer server.Close()
					baseURL := server.URL + "/"
					client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&baseURL, nil))
					require.NoError(t, err)
					loads := 0
					h := &PullRequestReview{Base: Base{
						AppName:       "policy-bot",
						ClientCreator: testClients{client: client, v4client: githubv4.NewEnterpriseClient(server.URL+"/graphql", server.Client())},
						BaseConfig:    &baseapp.HTTPConfig{PublicURL: "https://policy-bot.example.com"},
						PullOpts:      &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
						ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
							loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
								loads++
								return appconfig.Config{Content: []byte(source.child), Source: "example/service@main", Path: ".policy.yml"}, nil
							},
							loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
								return appconfig.Config{Content: []byte(source.parent), Source: "example/shared@main", Path: ".policy.yml"}, nil
							},
						}},
					}}
					event := github.PullRequestReviewEvent{
						Action: &test.action, Repo: repo, PullRequest: pr, Installation: &github.Installation{ID: new(int64(1))},
						Sender: &github.User{Login: &test.sender},
						Review: &github.PullRequestReview{State: &test.state, Body: &test.body, User: &github.User{Login: new("reviewer")}},
					}
					if test.previous != "" {
						event.Changes = &github.EditChange{Body: &github.EditBody{From: &test.previous}}
					}
					payload, err := json.Marshal(event)
					require.NoError(t, err)
					require.NoError(t, h.Handle(context.Background(), "pull_request_review", "delivery", payload))
					if test.status == "" {
						assert.Empty(t, statuses)
						assert.Empty(t, queries)
					} else {
						require.Len(t, statuses, 1)
						assert.Equal(t, test.status, (<-statuses).GetState())
						assert.Len(t, queries, 1)
					}
					if test.sender == "policy-bot[bot]" {
						assert.Zero(t, loads)
					} else {
						assert.Equal(t, 1, loads)
					}
				})
			}
		})
	}
}
