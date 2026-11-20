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
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/internal/responsetest"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestsForHead(t *testing.T) {
	tests := map[string]struct {
		responses []responsetest.SavedResponse
		want      []int
		wantError bool
	}{
		"filters closed historical and foreign associations across pages": {
			responses: []responsetest.SavedResponse{
				{Status: 200, Headers: map[string]string{"Link": `<https://api.github.com/repos/owner/repo/commits/head/pulls?page=2>; rel="next"`}, Body: `[{"number":1,"state":"open","head":{"sha":"head"},"base":{"repo":{"id":1}}},{"number":2,"state":"closed","head":{"sha":"head"},"base":{"repo":{"id":1}}},{"number":3,"state":"open","head":{"sha":"old"},"base":{"repo":{"id":1}}},{"number":4,"state":"open","head":{"sha":"head"},"base":{"repo":{"id":9}}}]`},
				{Status: 200, Body: `[{"number":5,"state":"open","head":{"sha":"head"},"base":{"repo":{"id":1}}}]`},
			}, want: []int{1, 5}},
		"no associations": {responses: []responsetest.SavedResponse{{Status: 200, Body: `[]`}}},
		"lookup failure":  {responses: []responsetest.SavedResponse{{Status: 403, Body: `{}`}}, wantError: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rp := &responsetest.ResponsePlayer{}
			rule := rp.AddResponse(responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"), tt.responses...)
			client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: rp}))
			require.NoError(t, err)
			repo := &github.Repository{ID: new(int64(1)), Name: new("repo"), Owner: &github.User{Login: new("owner")}}
			prs, err := pullRequestsForHead(context.Background(), client, repo, "head")
			if tt.wantError {
				require.ErrorContains(t, err, "failed to list pull requests")
			} else {
				require.NoError(t, err)
			}
			var numbers []int
			for _, pr := range prs {
				numbers = append(numbers, pr.GetNumber())
			}
			assert.Equal(t, tt.want, numbers, "only current open PRs in this repository should match")
			assert.Equal(t, len(tt.responses), rule.Count, "all pages should be read")
		})
	}
}

func TestCompletionPullRequestsSkipsLookup(t *testing.T) {
	prs := []*github.PullRequest{{Number: new(1)}}
	for name, tt := range map[string]struct {
		sha string
		prs []*github.PullRequest
	}{
		"associated": {sha: "head", prs: prs}, "missing SHA": {},
	} {
		t.Run(name, func(t *testing.T) {
			// A zero Base fails if a client is requested.
			got, err := (&Base{}).completionPullRequests(context.Background(), 0, nil, tt.sha, tt.prs)
			require.NoError(t, err)
			assert.Equal(t, tt.prs, got, "existing associations should be preserved")
		})
	}
}

type completionClientCreator struct {
	githubapp.ClientCreator
	client *github.Client
	v4     *githubv4.Client
	err    error
}

func (c completionClientCreator) NewInstallationClient(int64) (*github.Client, error) {
	return c.client, c.err
}
func (c completionClientCreator) NewInstallationV4Client(int64) (*githubv4.Client, error) {
	return c.v4, c.err
}

func TestCompletionPullRequestsLookup(t *testing.T) {
	rp := &responsetest.ResponsePlayer{}
	rule := rp.AddResponse(responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"), responsetest.SavedResponse{Status: 200, Body: `[{"number":1,"state":"open","head":{"sha":"head"},"base":{"repo":{"id":2}}}]`})
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: rp}))
	require.NoError(t, err)
	b := Base{ClientCreator: completionClientCreator{client: client}}
	repo := &github.Repository{ID: new(int64(2)), Name: new("repo"), Owner: &github.User{Login: new("owner")}}
	for range 2 {
		prs, err := b.completionPullRequests(context.Background(), 1, repo, "head", nil)
		require.NoError(t, err)
		if assert.Len(t, prs, 1, "matching PR should be returned") {
			assert.Equal(t, 1, prs[0].GetNumber(), "matching PR number")
		}
	}
	assert.Equal(t, 2, rule.Count, "each completion should perform a fresh lookup")
	b.ClientCreator = completionClientCreator{err: errors.New("client failure")}
	_, err = b.completionPullRequests(context.Background(), 1, repo, "other", nil)
	require.ErrorContains(t, err, "client failure")
}
