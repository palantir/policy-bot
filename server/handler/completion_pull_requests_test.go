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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/require"
)

func TestPullRequestsForHead(t *testing.T) {
	repo := &github.Repository{ID: new(int64(1)), Name: new("repo"), Owner: &github.User{Login: new("owner")}}
	pr := func(number int, state, sha string, baseID int64) *github.PullRequest {
		return &github.PullRequest{Number: &number, State: &state, Head: &github.PullRequestBranch{SHA: &sha, Repo: &github.Repository{ID: new(int64(2))}}, Base: &github.PullRequestBranch{Repo: &github.Repository{ID: &baseID}}}
	}
	t.Run("filters stale closed and foreign PRs across pages", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, "/repos/owner/repo/commits/head/pulls", r.URL.Path)
			require.Equal(t, "100", r.URL.Query().Get("per_page"))
			w.Header().Set("Content-Type", "application/json")
			if calls == 1 {
				w.Header().Set("Link", "<http://"+r.Host+r.URL.Path+"?page=2>; rel=\"next\"")
				require.NoError(t, json.NewEncoder(w).Encode([]*github.PullRequest{pr(1, "open", "head", 1), pr(2, "closed", "head", 1), pr(3, "open", "old", 1), pr(4, "open", "head", 9)}))
			} else {
				require.Equal(t, "2", r.URL.Query().Get("page"))
				require.NoError(t, json.NewEncoder(w).Encode([]*github.PullRequest{pr(5, "open", "head", 1)}))
			}
		}))
		defer server.Close()
		client, clientErr := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(new(server.URL+"/"), nil))
		require.NoError(t, clientErr)
		matches, err := pullRequestsForHead(context.Background(), client, repo, "head")
		require.NoError(t, err)
		require.Len(t, matches, 2)
		require.Equal(t, 1, matches[0].GetNumber())
		require.Equal(t, 5, matches[1].GetNumber())
		require.Equal(t, 2, calls)
	})
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte("[]"))
				}
			}))
			defer server.Close()
			client, clientErr := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(new(server.URL+"/"), nil))
			require.NoError(t, clientErr)
			matches, err := pullRequestsForHead(context.Background(), client, repo, "head")
			if status == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "failed to list pull requests")
			}
			require.Empty(t, matches)
		})
	}
}

func TestCompletionPullRequestsSkipsLookup(t *testing.T) {
	b := &Base{}
	prs := []*github.PullRequest{{Number: new(1)}}
	got, err := b.completionPullRequests(context.Background(), 0, nil, "head", prs)
	require.NoError(t, err)
	require.Equal(t, prs, got)
	got, err = b.completionPullRequests(context.Background(), 0, nil, "", nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
