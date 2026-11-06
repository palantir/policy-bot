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
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueCommentAuditsExtension(t *testing.T) {
	repo := &github.Repository{ID: new(int64(1)), Name: new("service"), Owner: &github.User{Login: new("example")}}
	pr := &github.PullRequest{
		Title: new("Change"), State: new("open"), Number: new(1), Draft: new(false),
		CreatedAt: &github.Timestamp{Time: time.Unix(1, 0)}, User: &github.User{Login: new("author")},
		Base:         &github.PullRequestBranch{Ref: new("main"), Repo: repo},
		Head:         &github.PullRequestBranch{Ref: new("feature"), SHA: new("head"), Repo: repo},
		ChangedFiles: new(1),
	}
	defaults := &approval.Defaults{Options: &approval.Options{
		Methods: &common.Methods{Comments: []string{"ship"}},
	}}
	for _, test := range []struct {
		name, input, body, status string
		defaults                  *approval.Defaults
		parent                    string
		action, sender            string
	}{
		{"parent default", extensionPolicy, "ship", "failure", nil, slotPolicy + "\napproval_defaults: {options: {methods: {comments: [ship]}}}", "edited", "editor"},
		{"parent without comment trigger", extensionPolicy, "ship", "failure", defaults, strings.Replace(slotPolicy, "count: 1", "count: 0", 1), "edited", "editor"},
		{"child override", extensionPolicy + childSlotFill, "child", "failure", nil, slotPolicy, "edited", "editor"},
		{"replaced parent method", extensionPolicy + childSlotFill, ":+1:", "", nil, slotPolicy, "edited", "editor"},
		{"missing required", extensionPolicy, ":+1:", "error", nil, "policy: {approval: [{slot: missing, required: true}]}", "edited", "editor"},
		{"invalid parent yaml", extensionPolicy, ":+1:", "error", nil, "approval_rules: [", "edited", "editor"},
		{"created missing required", extensionPolicy, ":+1:", "error", nil, "policy: {approval: [{slot: missing, required: true}]}", "created", "reviewer"},
		{"author edit missing required", extensionPolicy, ":+1:", "error", nil, "policy: {approval: [{slot: missing, required: true}]}", "edited", "reviewer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			statuses := make(chan *github.RepoStatus, 4)
			client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /repos/example/service/pulls/1":
					assert.NoError(t, json.NewEncoder(w).Encode(pr))
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
			})
			h := &IssueComment{Base: Base{
				ClientCreator: testClients{client: client},
				BaseConfig:    &baseapp.HTTPConfig{PublicURL: "https://policy-bot.example.com"},
				PullOpts:      &PullEvaluationOptions{StatusCheckContext: "policy-bot", ApprovalDefaults: test.defaults},
				ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
					loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
						return appconfig.Config{Content: []byte(test.input), Source: "example/service@main", Path: ".policy.yml"}, nil
					},
					loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
						return appconfig.Config{Content: []byte(test.parent), Source: "example/shared@main", Path: ".policy.yml"}, nil
					},
				}},
			}}
			event := github.IssueCommentEvent{
				Action: &test.action, Repo: repo, Installation: &github.Installation{ID: new(int64(1))},
				Issue:   &github.Issue{Number: new(1), PullRequestLinks: &github.PullRequestLinks{}},
				Sender:  &github.User{Login: &test.sender},
				Comment: &github.IssueComment{User: &github.User{Login: new("reviewer")}, Body: new("removed")},
				Changes: &github.EditChange{Body: &github.EditBody{From: &test.body}},
			}
			payload, err := json.Marshal(event)
			require.NoError(t, err)
			err = h.Handle(context.Background(), "issue_comment", "delivery", payload)
			if test.status == "error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if test.status == "" {
				assert.Empty(t, statuses)
			} else {
				require.Len(t, statuses, 1)
				assert.Equal(t, test.status, (<-statuses).GetState())
			}
		})
	}
}
