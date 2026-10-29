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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs"
	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"goji.io"
	"goji.io/pat"
)

func TestDetailsReviewersExtension(t *testing.T) {
	const parent = `
policy:
  approval: [Review, {slot: service}, {slot: default}, {slot: empty}]
slots: {default: [Review]}
approval_rules:
  - name: Review
    requires: {count: 1, users: [parent-reviewer]}
`
	const child = extensionPolicy + `
slots: {service: [Review]}
approval_rules:
  - name: Review
    requires: {count: 1, users: [child-reviewer]}
`
	repo := &github.Repository{ID: new(int64(1)), Name: new("service"), Owner: &github.User{Login: new("example")}}
	pr := &github.PullRequest{
		Title: new("Change"), State: new("open"), Number: new(1), Draft: new(false),
		CreatedAt: &github.Timestamp{Time: time.Unix(1, 0)}, User: &github.User{Login: new("author")},
		Base:         &github.PullRequestBranch{Ref: new("main"), Repo: repo},
		Head:         &github.PullRequestBranch{Ref: new("feature"), SHA: new("head"), Repo: repo},
		ChangedFiles: new(1),
	}
	templates, err := LoadTemplates(&FilesConfig{Templates: "../templates", Static: t.TempDir()}, "", "https://github.com")
	require.NoError(t, err)
	sessions := scs.NewCookieManager("test-session-key")
	login := httptest.NewRecorder()
	require.NoError(t, sessions.Load(httptest.NewRequest(http.MethodGet, "/", nil)).PutString(login, SessionKeyUsername, "reader"))
	for _, test := range []struct {
		name, slot, child, parent, permission, reviewer string
		loadError, incomplete                           bool
	}{
		{"base", "", child, parent, "read", "parent-reviewer", false, false},
		{"child fill", "service", child, parent, "read", "child-reviewer", false, false},
		{"default fill", "default", child, parent, "read", "parent-reviewer", false, false},
		{"empty slot", "empty", child, parent, "read", "", false, false},
		{"unknown scope", "unknown", child, parent, "read", "", false, false},
		{"standalone name", "", parent, "", "read", "parent-reviewer", false, false},
		{"null rule", "", "approval_rules: [null]", "", "read", "", false, true},
		{"bad parent", "service", child, "approval_rules: [", "read", "", false, true},
		{"null parent rule", "service", child, "approval_rules: [null]", "read", "", false, true},
		{"parent unavailable", "service", child, "", "read", "", true, true},
		{"denied", "service", child, parent, "none", "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /repos/example/service/collaborators/reader/permission":
					assert.NoError(t, json.NewEncoder(w).Encode(github.RepositoryPermissionLevel{Permission: &test.permission}))
				case "GET /repos/example/service/pulls/1":
					assert.NotEqual(t, "none", test.permission)
					assert.NoError(t, json.NewEncoder(w).Encode(pr))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&baseURL, nil))
			require.NoError(t, err)
			h := &DetailsReviewers{Details: Details{
				Sessions: sessions, Templates: templates,
				Base: Base{
					ClientCreator: testClients{client: client}, Installations: reviewerInstallations{},
					BaseConfig: &baseapp.HTTPConfig{PublicURL: "https://policy-bot.example.com"}, PullOpts: &PullEvaluationOptions{},
					ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
						loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
							return appconfig.Config{Content: []byte(test.child), Source: "example/service@main", Path: ".policy.yml"}, nil
						},
						loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
							if test.loadError {
								return appconfig.Config{}, errors.New("parent unavailable")
							}
							return appconfig.Config{Content: []byte(test.parent), Source: "example/shared@main", Path: ".policy.yml"}, nil
						},
					}},
				},
			}}
			mux := goji.NewMux()
			mux.HandleFunc(pat.Get("/details/:owner/:repo/:number/reviewers"), func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, h.ServeHTTP(w, r))
			})
			request := httptest.NewRequest(http.MethodGet, "/details/example/service/1/reviewers?rule=Review&slot="+test.slot, nil)
			request.Header.Set("HX-Request", "true")
			for _, cookie := range login.Result().Cookies() {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if test.permission == "none" {
				assert.Equal(t, http.StatusNotFound, response.Code)
				return
			}
			assert.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			assert.Equal(t, test.incomplete, strings.Contains(body, "reviewer list may be incomplete"))
			if test.reviewer == "" {
				assert.Contains(t, body, "No reviewers found")
			} else {
				assert.Contains(t, body, ">"+test.reviewer+"</a>")
			}
		})
	}
}

type reviewerInstallations struct{ githubapp.InstallationsService }

func (reviewerInstallations) GetByOwner(context.Context, string) (githubapp.Installation, error) {
	return githubapp.Installation{ID: 1}, nil
}
