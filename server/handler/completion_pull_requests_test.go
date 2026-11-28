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
		responseFile  string
		expectedPages int
		want          []int
		wantError     bool
	}{
		"filters closed historical and foreign associations on first page": {
			responseFile: "completion_pull_requests_paginated.yml", expectedPages: 1, want: []int{1},
		},
		"no associations": {responseFile: "completion_pull_requests_empty.yml", expectedPages: 1},
		"lookup failure":  {responseFile: "completion_pull_requests_forbidden.yml", expectedPages: 1, wantError: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rp := &responsetest.ResponsePlayer{}
			rule := rp.AddRule(
				responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"),
				"testdata/responses/"+tt.responseFile,
			)
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
			assert.Equal(t, tt.expectedPages, rule.Count, "only the first page should be read")
		})
	}
}

func TestWorkflowRunFindPullRequestsForHeadSkipsMissingSHA(t *testing.T) {
	got, err := (&WorkflowRun{}).findPullRequestsForHead(context.Background(), 0, nil, "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

type mockCompletionClientCreator struct {
	githubapp.ClientCreator
	client *github.Client
	v4     *githubv4.Client
	err    error
}

func (c mockCompletionClientCreator) NewInstallationClient(int64) (*github.Client, error) {
	return c.client, c.err
}

func (c mockCompletionClientCreator) NewInstallationV4Client(int64) (*githubv4.Client, error) {
	return c.v4, c.err
}

func TestCompletionPullRequestsLookup(t *testing.T) {
	rp := &responsetest.ResponsePlayer{}
	rule := rp.AddRule(
		responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"),
		"testdata/responses/completion_pull_requests.yml",
	)
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: rp}))
	require.NoError(t, err)
	b := WorkflowRun{Base: Base{ClientCreator: mockCompletionClientCreator{client: client}}}
	repo := &github.Repository{ID: new(int64(1)), Name: new("repo"), Owner: &github.User{Login: new("owner")}}
	for range 2 {
		prs, err := b.findPullRequestsForHead(context.Background(), 1, repo, "head")
		require.NoError(t, err)
		if assert.Len(t, prs, 1, "matching PR should be returned") {
			assert.Equal(t, 42, prs[0].GetNumber(), "matching PR number")
		}
	}
	assert.Equal(t, 2, rule.Count, "each completion should perform a fresh lookup")
	b.ClientCreator = mockCompletionClientCreator{err: errors.New("client failure")}
	_, err = b.findPullRequestsForHead(context.Background(), 1, repo, "other")
	require.ErrorContains(t, err, "client failure")
}

func TestPullRequestsForCommit(t *testing.T) {
	rp := &responsetest.ResponsePlayer{}
	rule := rp.AddRule(
		responsetest.ExactPathMatcher("/repos/owner/repo/commits/head/pulls"),
		"testdata/responses/completion_pull_requests_paginated.yml",
	)
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: rp}))
	require.NoError(t, err)
	repo := &github.Repository{ID: new(int64(1)), Name: new("repo"), Owner: &github.User{Login: new("owner")}}
	prs, err := pullRequestsForCommit(context.Background(), client, repo, "head")
	require.NoError(t, err)

	// Status events also need historical associations. Filtering belongs to callers.
	var numbers []int
	for _, pr := range prs {
		numbers = append(numbers, pr.GetNumber())
	}
	assert.Equal(t, []int{1, 2, 3, 4}, numbers, "shared lookup must preserve all associations")
	assert.Equal(t, 1, rule.Count, "only the first page should be read")
}
