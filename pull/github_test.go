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

package pull

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthor(t *testing.T) {
	endpoint := "/repos/testorg/testrepo/pulls/123"

	rp := NewResponsePlayer(map[string]string{
		endpoint: "testdata/responses/pull_author.yml",
	})
	ctx := makeContext(rp)

	author, err := ctx.Author()
	require.NoError(t, err)

	assert.Equal(t, "mhaypenny", author)
	assert.Equal(t, 1, rp.Requests[endpoint], "no http request was made")

	author, err = ctx.Author()
	require.NoError(t, err)

	// verify that the pull request is cached
	assert.Equal(t, "mhaypenny", author)
	assert.Equal(t, 1, rp.Requests[endpoint], "cached pull request was not used")
}

func TestChangedFiles(t *testing.T) {
	endpoint := "/repos/testorg/testrepo/pulls/123/files"

	rp := NewResponsePlayer(map[string]string{
		endpoint: "testdata/responses/pull_files.yml",
	})
	ctx := makeContext(rp)

	files, err := ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 3, "incorrect number of files")
	assert.Equal(t, 2, rp.Requests[endpoint], "no http request was made")

	assert.Equal(t, "path/foo.txt", files[0].Filename)
	assert.Equal(t, FileAdded, files[0].Status)

	assert.Equal(t, "path/bar.txt", files[1].Filename)
	assert.Equal(t, FileDeleted, files[1].Status)

	assert.Equal(t, "README.md", files[2].Filename)
	assert.Equal(t, FileModified, files[2].Status)

	// verify that the file list is cached
	files, err = ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 3, "incorrect number of files")
	assert.Equal(t, 2, rp.Requests[endpoint], "cached files were not used")
}

func TestCommits(t *testing.T) {
	endpointTimeline := "/repos/testorg/testrepo/issues/123/timeline"
	endpointCommits := "/repos/testorg/testrepo/pulls/123/commits"

	rp := NewResponsePlayer(map[string]string{
		endpointTimeline: "testdata/responses/timeline_commits.yml",
		endpointCommits:  "testdata/responses/pull_commits.yml",
	})
	ctx := makeContext(rp)

	commits, err := ctx.Commits()
	require.NoError(t, err)

	require.Len(t, commits, 3, "incorrect number of commits")
	assert.Equal(t, 2, rp.Requests[endpointTimeline], "no http request was made to timeline")
	assert.Equal(t, 2, rp.Requests[endpointCommits], "no http request was made to commits")

	assert.Equal(t, "a6f3f69b64eaafece5a0d854eb4af11c0d64394c", commits[0].SHA)
	assert.Equal(t, "mhaypenny", commits[0].Author)
	assert.Equal(t, "mhaypenny", commits[0].Committer)
	assert.Equal(t, 0, commits[0].Order)

	assert.Equal(t, "1fc89f1cedf8e3f3ce516ab75b5952295c8ea5e9", commits[1].SHA)
	assert.Equal(t, "mhaypenny", commits[1].Author)
	assert.Equal(t, "mhaypenny", commits[1].Committer)
	assert.Equal(t, 1, commits[1].Order)

	assert.Equal(t, "e05fcae367230ee709313dd2720da527d178ce43", commits[2].SHA)
	assert.Equal(t, "ttest", commits[2].Author)
	assert.Equal(t, "mhaypenny", commits[2].Committer)
	assert.Equal(t, 2, commits[2].Order)

	// verify that the commit list is cached
	commits, err = ctx.Commits()
	require.NoError(t, err)

	require.Len(t, commits, 3, "incorrect number of commits")
	assert.Equal(t, 2, rp.Requests[endpointTimeline], "cached commits were not used for timeline")
	assert.Equal(t, 2, rp.Requests[endpointCommits], "cached commits were not used for commits")
}

func TestReviews(t *testing.T) {
	endpoint := "/repos/testorg/testrepo/issues/123/timeline"

	rp := NewResponsePlayer(map[string]string{
		endpoint: "testdata/responses/timeline_review.yml",
	})
	ctx := makeContext(rp)

	reviews, err := ctx.Reviews()
	require.NoError(t, err)

	require.Len(t, reviews, 1, "incorrect number of reviews")
	assert.Equal(t, 1, rp.Requests[endpoint], "no http request was made")

	expectedTime, err := time.Parse(time.RFC3339, "2018-06-27T20:33:26Z")
	assert.NoError(t, err)

	assert.Equal(t, 0, reviews[0].Order)
	assert.Equal(t, "bkeyes", reviews[0].Author)
	assert.Equal(t, expectedTime, reviews[0].LastModified)
	assert.Equal(t, ReviewApproved, reviews[0].State)
	assert.Equal(t, "the body", reviews[0].Body)

	// verify that the review list is cached
	reviews, err = ctx.Reviews()
	require.NoError(t, err)

	require.Len(t, reviews, 1, "incorrect number of reviews")
	assert.Equal(t, 1, rp.Requests[endpoint], "cached reviews were not used")
}

func TestComments(t *testing.T) {
	endpoint := "/repos/testorg/testrepo/issues/123/timeline"

	rp := NewResponsePlayer(map[string]string{
		endpoint: "testdata/responses/timeline_comments.yml",
	})
	ctx := makeContext(rp)

	comments, err := ctx.Comments()
	require.NoError(t, err)

	require.Len(t, comments, 1, "incorrect number of comments")
	assert.Equal(t, 1, rp.Requests[endpoint], "no http request was made")

	expectedTime, err := time.Parse(time.RFC3339, "2018-06-27T20:28:22Z")
	assert.NoError(t, err)

	assert.Equal(t, 0, comments[0].Order)
	assert.Equal(t, "bkeyes", comments[0].Author)
	assert.Equal(t, expectedTime, comments[0].LastModified)
	assert.Equal(t, ":+1:", comments[0].Body)

	// verify that the commit list is cached
	comments, err = ctx.Comments()
	require.NoError(t, err)

	require.Len(t, comments, 1, "incorrect number of comments")
	assert.Equal(t, 1, rp.Requests[endpoint], "cached comments were not used")
}

func TestIsTeamMember(t *testing.T) {
	endpointTeams := "/orgs/testorg/teams"
	endpointYes1 := "/teams/123/memberships/mhaypenny"
	endpointYes2 := "/teams/123/memberships/ttest"
	endpointNo1 := "/teams/456/memberships/mhaypenny"
	endpointNo2 := "/teams/456/memberships/ttest"

	rp := NewResponsePlayer(map[string]string{
		endpointTeams: "testdata/responses/teams_testorg.yml",
		endpointYes1:  "testdata/responses/membership_team123_mhaypenny.yml",
		endpointYes2:  "testdata/responses/membership_team123_ttest.yml",
		endpointNo1:   "testdata/responses/membership_team456_mhaypenny.yml",
		endpointNo2:   "testdata/responses/membership_team456_ttest.yml",
	})
	ctx := makeContext(rp)

	isMember, err := ctx.IsTeamMember("testorg/yes-team", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 2, rp.Requests[endpointTeams], "no http request was made for teams")
	assert.Equal(t, 1, rp.Requests[endpointYes1], "no http request was made")

	isMember, err = ctx.IsTeamMember("testorg/yes-team", "ttest")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 2, rp.Requests[endpointTeams], "cached team IDs were not used")
	assert.Equal(t, 1, rp.Requests[endpointYes2], "no http request was made")

	// not a member because missing from team
	isMember, err = ctx.IsTeamMember("testorg/no-team", "mhaypenny")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 2, rp.Requests[endpointTeams], "cached team IDs were not used")
	assert.Equal(t, 1, rp.Requests[endpointNo1], "no http request was made")

	// not a member because membership state is pending
	isMember, err = ctx.IsTeamMember("testorg/no-team", "ttest")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 2, rp.Requests[endpointTeams], "cached team IDs were not used")
	assert.Equal(t, 1, rp.Requests[endpointNo2], "no http request was made")

	// verify that team membership is cached
	isMember, err = ctx.IsTeamMember("testorg/yes-team", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 2, rp.Requests[endpointTeams], "cached team IDs were not used")
	assert.Equal(t, 1, rp.Requests[endpointYes1], "cached membership was not used")
}

func TestIsOrgMember(t *testing.T) {
	endpointYes := "/orgs/testorg/members/mhaypenny"
	endpointNo := "/orgs/testorg/members/ttest"

	rp := NewResponsePlayer(map[string]string{
		endpointYes: "testdata/responses/membership_testorg_mhaypenny.yml",
		endpointNo:  "testdata/responses/membership_testorg_ttest.yml",
	})
	ctx := makeContext(rp)

	isMember, err := ctx.IsOrgMember("testorg", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, rp.Requests[endpointYes], "no http request was made")

	isMember, err = ctx.IsOrgMember("testorg", "ttest")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 1, rp.Requests[endpointNo], "no http request was made")

	// verify that org membership is cached
	isMember, err = ctx.IsOrgMember("testorg", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, rp.Requests[endpointYes], "cached membership was not used")
}

func makeContext(rp *ResponsePlayer) Context {
	ctx := context.Background()
	client := github.NewClient(&http.Client{Transport: rp})
	v4client := githubv4.NewClient(&http.Client{Transport: rp})

	base, _ := url.Parse("http://github.localhost/")
	client.BaseURL = base

	mbrCtx := NewGitHubMembershipContext(ctx, client)
	pr, _, _ := client.PullRequests.Get(ctx, "testorg", "testrepo", 123)
	if pr == nil {
		// create a stub PR if none is returned from the response player
		pr = &github.PullRequest{}
	}

	// insert the values needed for the context
	repoOwner, repoName, prNum := "testorg", "testrepo", 123
	pr.Number = &prNum
	pr.Base = &github.PullRequestBranch{
		Repo: &github.Repository{
			Owner: &github.User{
				Login: &repoOwner,
			},
			Name: &repoName,
		},
	}

	return NewGitHubContext(ctx, mbrCtx, client, v4client, pr)
}
