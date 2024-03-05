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
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangedFiles(t *testing.T) {
	rp := &ResponsePlayer{}
	filesRule := rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/pulls/123/files"),
		"testdata/responses/pull_files.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	files, err := ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 5, "incorrect number of files")
	assert.Equal(t, 2, filesRule.Count, "no http request was made")

	assert.Equal(t, "path/foo.txt", files[0].Filename)
	assert.Equal(t, FileAdded, files[0].Status)

	assert.Equal(t, "path/bar.txt", files[1].Filename)
	assert.Equal(t, FileDeleted, files[1].Status)

	assert.Equal(t, "README.md", files[2].Filename)
	assert.Equal(t, FileModified, files[2].Status)

	assert.Equal(t, "path/old.txt", files[3].Filename)
	assert.Equal(t, FileDeleted, files[3].Status)
	assert.Equal(t, 0, files[3].Additions)
	assert.Equal(t, 0, files[3].Deletions)

	assert.Equal(t, "path/new.txt", files[4].Filename)
	assert.Equal(t, FileAdded, files[4].Status)
	assert.Equal(t, 2, files[4].Additions)
	assert.Equal(t, 4, files[4].Deletions)

	// verify that the file list is cached
	files, err = ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 5, "incorrect number of files")
	assert.Equal(t, 2, filesRule.Count, "cached files were not used")
}

func TestChangedFilesNoFiles(t *testing.T) {
	rp := &ResponsePlayer{}
	filesRule := rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/pulls/123/files"),
		"testdata/responses/pull_no_files.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	files, err := ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 0, "incorrect number of files")
	assert.Equal(t, 1, filesRule.Count, "no http request was made")

	// verify that the file list is cached
	files, err = ctx.ChangedFiles()
	require.NoError(t, err)

	require.Len(t, files, 0, "incorrect number of files")
	assert.Equal(t, 1, filesRule.Count, "cached files were not used")
}

func TestCommits(t *testing.T) {
	rp := &ResponsePlayer{}
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.commits"),
		"testdata/responses/pull_commits.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	commits, err := ctx.Commits()
	require.NoError(t, err)

	require.Len(t, commits, 3, "incorrect number of commits")
	assert.Equal(t, 2, dataRule.Count, "incorrect number of http requests")

	assert.Equal(t, "a6f3f69b64eaafece5a0d854eb4af11c0d64394c", commits[0].SHA)
	assert.Equal(t, "mhaypenny", commits[0].Author)
	assert.Equal(t, "mhaypenny", commits[0].Committer)
	assert.Nil(t, commits[0].Signature)

	assert.Equal(t, "1fc89f1cedf8e3f3ce516ab75b5952295c8ea5e9", commits[1].SHA)
	assert.Equal(t, "mhaypenny", commits[1].Author)
	assert.Equal(t, "mhaypenny", commits[1].Committer)
	assert.Nil(t, commits[1].Signature)

	assert.Equal(t, "e05fcae367230ee709313dd2720da527d178ce43", commits[2].SHA)
	assert.Equal(t, "ttest", commits[2].Author)
	assert.Equal(t, "mhaypenny", commits[2].Committer)

	// verify that the signature was handled correctly
	assert.NotNil(t, commits[2].Signature)
	assert.Equal(t, "3AA5C34371567BD2", commits[2].Signature.KeyID)
	assert.Equal(t, "mhaypenny", commits[2].Signature.Signer)
	assert.True(t, commits[2].Signature.IsValid)

	// verify that the commit list is cached
	commits, err = ctx.Commits()
	require.NoError(t, err)

	require.Len(t, commits, 3, "incorrect number of commits")
	assert.Equal(t, 2, dataRule.Count, "cached commits were not used")
}

func TestReviews(t *testing.T) {
	rp := &ResponsePlayer{}
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.reviews"),
		"testdata/responses/pull_reviews.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	reviews, err := ctx.Reviews()
	require.NoError(t, err)

	require.Len(t, reviews, 3, "incorrect number of reviews")
	assert.Equal(t, 2, dataRule.Count, "no http request was made")

	expectedTime, err := time.Parse(time.RFC3339, "2018-06-27T20:33:26Z")
	assert.NoError(t, err)

	assert.Equal(t, "mhaypenny", reviews[0].Author)
	assert.Equal(t, expectedTime, reviews[0].CreatedAt)
	assert.Equal(t, expectedTime, reviews[0].LastEditedAt)
	assert.Equal(t, ReviewChangesRequested, reviews[0].State)
	assert.Equal(t, "", reviews[0].Body)

	assert.Equal(t, "bkeyes", reviews[1].Author)
	assert.Equal(t, expectedTime.Add(time.Second), reviews[1].CreatedAt)
	assert.Equal(t, expectedTime.Add(time.Second), reviews[1].LastEditedAt)
	assert.Equal(t, ReviewApproved, reviews[1].State)
	assert.Equal(t, "the body", reviews[1].Body)

	assert.Equal(t, "jgiannuzzi", reviews[2].Author)
	assert.Equal(t, expectedTime.Add(-4*time.Second).Add(5*time.Minute), reviews[2].CreatedAt)
	assert.Equal(t, expectedTime.Add(-4*time.Second).Add(5*time.Minute), reviews[2].LastEditedAt)
	assert.Equal(t, ReviewCommented, reviews[2].State)
	assert.Equal(t, "A review comment", reviews[2].Body)

	// verify that the review list is cached
	reviews, err = ctx.Reviews()
	require.NoError(t, err)

	require.Len(t, reviews, 3, "incorrect number of reviews")
	assert.Equal(t, 2, dataRule.Count, "cached reviews were not used")
}

func TestNoReviews(t *testing.T) {
	rp := &ResponsePlayer{}
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.reviews"),
		"testdata/responses/pull_no_reviews.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	reviews, err := ctx.Reviews()
	require.NoError(t, err)
	require.Empty(t, reviews, "incorrect number of reviews")

	// verify that the review list is cached
	reviews, err = ctx.Reviews()
	require.NoError(t, err)

	assert.Empty(t, reviews, "incorrect number of reviews")
	assert.Equal(t, 1, dataRule.Count, "cached reviews were not used")
}

func TestBody(t *testing.T) {
	rp := &ResponsePlayer{}
	rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest"),
		"testdata/responses/pull_body.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	prBody, err := ctx.Body()
	require.NoError(t, err)

	expectedTime, err := time.Parse(time.RFC3339, "2018-01-12T00:11:50Z")
	assert.NoError(t, err)

	assert.Equal(t, "/no-platform", prBody.Body)
	assert.Equal(t, "agirlnamedsophia", prBody.Author)
	assert.Equal(t, expectedTime, prBody.CreatedAt)
	assert.Equal(t, expectedTime, prBody.LastEditedAt)
}

func TestComments(t *testing.T) {
	rp := &ResponsePlayer{}
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.comments"),
		"testdata/responses/pull_comments.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	comments, err := ctx.Comments()
	require.NoError(t, err)

	require.Len(t, comments, 3, "incorrect number of comments")
	assert.Equal(t, 2, dataRule.Count, "no http request was made")

	expectedTime, err := time.Parse(time.RFC3339, "2018-06-27T20:28:22Z")
	assert.NoError(t, err)

	assert.Equal(t, "bkeyes", comments[0].Author)
	assert.Equal(t, expectedTime, comments[0].CreatedAt)
	assert.Equal(t, expectedTime, comments[0].LastEditedAt)
	assert.Equal(t, ":+1:", comments[0].Body)

	assert.Equal(t, "bulldozer[bot]", comments[1].Author)
	assert.Equal(t, expectedTime.Add(time.Minute), comments[1].CreatedAt)
	assert.Equal(t, expectedTime.Add(time.Minute), comments[1].LastEditedAt)
	assert.Equal(t, "I merge!", comments[1].Body)

	assert.Equal(t, "jgiannuzzi", comments[2].Author)
	assert.Equal(t, expectedTime.Add(10*time.Minute), comments[2].CreatedAt)
	assert.Equal(t, expectedTime.Add(10*time.Minute), comments[2].LastEditedAt)
	assert.Equal(t, "A review comment", comments[2].Body)

	// verify that the commit list is cached
	comments, err = ctx.Comments()
	require.NoError(t, err)

	require.Len(t, comments, 3, "incorrect number of comments")
	assert.Equal(t, 2, dataRule.Count, "cached comments were not used")
}

func TestNoComments(t *testing.T) {
	rp := &ResponsePlayer{}
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.comments"),
		"testdata/responses/pull_no_comments.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	comments, err := ctx.Comments()
	require.NoError(t, err)
	require.Empty(t, comments, "incorrect number of comments")

	// verify that the commit list is cached
	comments, err = ctx.Comments()
	require.NoError(t, err)

	assert.Empty(t, comments, "incorrect number of comments")
	assert.Equal(t, 1, dataRule.Count, "cached comments were not used")
}

func TestIsTeamMember(t *testing.T) {
	rp := &ResponsePlayer{}
	yesRule1 := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/yes-team/memberships/mhaypenny"),
		"testdata/responses/membership_team123_mhaypenny.yml",
	)
	yesRule2 := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/yes-team/memberships/ttest"),
		"testdata/responses/membership_team123_ttest.yml",
	)
	noRule1 := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/no-team/memberships/mhaypenny"),
		"testdata/responses/membership_team456_mhaypenny.yml",
	)
	noRule2 := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/no-team/memberships/ttest"),
		"testdata/responses/membership_team456_ttest.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	isMember, err := ctx.IsTeamMember("testorg/yes-team", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, yesRule1.Count, "no http request was made")

	isMember, err = ctx.IsTeamMember("testorg/yes-team", "ttest")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, yesRule2.Count, "no http request was made")

	// not a member because missing from team
	isMember, err = ctx.IsTeamMember("testorg/no-team", "mhaypenny")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 1, noRule1.Count, "no http request was made")

	// not a member because membership state is pending
	isMember, err = ctx.IsTeamMember("testorg/no-team", "ttest")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 1, noRule2.Count, "no http request was made")

	// verify that team membership is cached
	isMember, err = ctx.IsTeamMember("testorg/yes-team", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, yesRule1.Count, "cached membership was not used")
}

func TestMixedReviewCommentPaging(t *testing.T) {
	rp := &ResponsePlayer{}
	rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/pulls/123"),
		"testdata/responses/pull.yml",
	)
	dataRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest"),
		"testdata/responses/pull_reviews_comments.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	comments, err := ctx.Comments()
	require.NoError(t, err)

	reviews, err := ctx.Reviews()
	require.NoError(t, err)

	assert.Equal(t, 2, dataRule.Count, "cached values were not used")
	assert.Len(t, comments, 3, "incorrect number of comments")
	assert.Len(t, reviews, 3, "incorrect number of reviews")
}

func TestIsOrgMember(t *testing.T) {
	rp := &ResponsePlayer{}
	yesRule := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/members/mhaypenny"),
		"testdata/responses/membership_testorg_mhaypenny.yml",
	)
	noRule := rp.AddRule(
		ExactPathMatcher("/orgs/testorg/members/ttest"),
		"testdata/responses/membership_testorg_ttest.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	isMember, err := ctx.IsOrgMember("testorg", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, yesRule.Count, "no http request was made")

	isMember, err = ctx.IsOrgMember("testorg", "ttest")
	require.NoError(t, err)

	assert.False(t, isMember, "user is a member")
	assert.Equal(t, 1, noRule.Count, "no http request was made")

	// verify that org membership is cached
	isMember, err = ctx.IsOrgMember("testorg", "mhaypenny")
	require.NoError(t, err)

	assert.True(t, isMember, "user is not a member")
	assert.Equal(t, 1, yesRule.Count, "cached membership was not used")
}

func TestBranches(t *testing.T) {
	rp := &ResponsePlayer{}
	ctx := makeContext(t, rp, nil, nil)

	base, head := ctx.Branches()
	assert.Equal(t, "develop", base, "base branch was not correctly set")
	assert.Equal(t, "test-branch", head, "head branch was not correctly set")
}

func TestCrossRepoBranches(t *testing.T) {
	rp := &ResponsePlayer{}

	// change the source repo to a forked repo
	crossRepoPr := defaultTestPR()
	crossRepoPr.Head.Repo = &github.Repository{
		ID: github.Int64(12345),
		Owner: &github.User{
			Login: github.String("testorg2"),
		},
		Name: github.String("testrepofork"),
	}

	ctx := makeContext(t, rp, crossRepoPr, nil)

	base, head := ctx.Branches()
	assert.Equal(t, "develop", base, "cross-repo base branch was not correctly set")
	assert.Equal(t, "testorg2:test-branch", head, "cross-repo head branch was not correctly set")
}

func TestCollaboratorPermission(t *testing.T) {
	rp := &ResponsePlayer{}
	rule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.collaborators"),
		"testdata/responses/repo_collaborator_permission.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	p, err := ctx.CollaboratorPermission("direct-admin")
	require.NoError(t, err)
	assert.Equal(t, PermissionAdmin, p, "incorrect permission for direct-admin")
	assert.Equal(t, 1, rule.Count, "incorrect http request count")

	rule.Count = 0

	p, err = ctx.CollaboratorPermission("direct")
	require.NoError(t, err)
	assert.Equal(t, PermissionNone, p, "incorrect permission for missing user")
	assert.Equal(t, 2, rule.Count, "incorrect http request count")

	p, err = ctx.CollaboratorPermission("direct-admin")
	require.NoError(t, err)
	assert.Equal(t, PermissionAdmin, p, "incorrect permission for direct-admin")
	assert.Equal(t, 2, rule.Count, "cached data was not used on second request")

	p, err = ctx.CollaboratorPermission("team-maintain")
	require.NoError(t, err)
	assert.Equal(t, PermissionMaintain, p, "incorrect permission for team-maintain")
}

func TestRepositoryCollaborators(t *testing.T) {
	rp := &ResponsePlayer{}
	rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/teams"),
		"testdata/responses/repo_teams.yml",
	)
	rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/maintainers/members"),
		"testdata/responses/repo_team_members_maintainers.yml",
	)
	rp.AddRule(
		ExactPathMatcher("/orgs/testorg/teams/admins/members"),
		"testdata/responses/repo_team_members_admins.yml",
	)
	rp.AddRule(
		GraphQLNodePrefixMatcher("repository.collaborators"),
		"testdata/responses/repo_collaborators.yml",
	)

	ctx := makeContext(t, rp, nil, nil)

	collaborators, err := ctx.RepositoryCollaborators()
	require.NoError(t, err)

	require.Len(t, collaborators, 8, "incorrect number of collaborators")
	sort.Slice(collaborators, func(i, j int) bool { return collaborators[i].Name < collaborators[j].Name })

	c0 := collaborators[0]
	assert.Equal(t, "direct-admin", c0.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionAdmin, ViaRepo: true},
	}, c0.Permissions)

	c1 := collaborators[1]
	assert.Equal(t, "direct-triage", c1.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionTriage, ViaRepo: true},
	}, c1.Permissions)

	c2 := collaborators[2]
	assert.Equal(t, "direct-write-team-maintain", c2.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionMaintain, ViaRepo: true},
		{Permission: PermissionWrite, ViaRepo: true},
	}, c2.Permissions)

	c3 := collaborators[3]
	assert.Equal(t, "org-owner", c3.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionAdmin, ViaRepo: false},
	}, c3.Permissions)

	c4 := collaborators[4]
	assert.Equal(t, "org-owner-team-maintain", c4.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionAdmin, ViaRepo: false},
		{Permission: PermissionMaintain, ViaRepo: true},
	}, c4.Permissions)

	c5 := collaborators[5]
	assert.Equal(t, "org-read", c5.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionRead, ViaRepo: false},
	}, c5.Permissions)

	c6 := collaborators[6]
	assert.Equal(t, "team-admin", c6.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionAdmin, ViaRepo: true},
	}, c6.Permissions)

	c7 := collaborators[7]
	assert.Equal(t, "team-maintain", c7.Name)
	assert.Equal(t, []CollaboratorPermission{
		{Permission: PermissionMaintain, ViaRepo: true},
	}, c7.Permissions)
}

func TestPushedAt(t *testing.T) {
	rp := &ResponsePlayer{}
	commitsRule := rp.AddRule(
		GraphQLNodePrefixMatcher("repository.pullRequest.commits"),
		"testdata/responses/pull_commits.yml",
	)
	statusRuleA6F := rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/commits/a6f3f69b64eaafece5a0d854eb4af11c0d64394c/statuses"),
		"testdata/responses/repo_statuses_none.yml",
	)
	statusRule1FC := rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/commits/1fc89f1cedf8e3f3ce516ab75b5952295c8ea5e9/statuses"),
		"testdata/responses/repo_statuses_none.yml",
	)
	statusRuleE05 := rp.AddRule(
		ExactPathMatcher("/repos/testorg/testrepo/commits/e05fcae367230ee709313dd2720da527d178ce43/statuses"),
		"testdata/responses/repo_statuses_e05fcae367230ee709313dd2720da527d178ce43.yml",
	)

	expectedTime := time.Date(2020, 9, 30, 17, 30, 0, 0, time.UTC)

	gc := NewMockGlobalCache()
	ctx := makeContext(t, rp, nil, gc)

	t.Run("fromStatus", func(t *testing.T) {
		pushedAt, err := ctx.PushedAt("e05fcae367230ee709313dd2720da527d178ce43")
		require.NoError(t, err)

		assert.Equal(t, expectedTime, pushedAt, "incorrect pushed at for commit")
		assert.Equal(t, 3, statusRuleE05.Count, "incorrect http request count")
	})

	t.Run("fromCache", func(t *testing.T) {
		pushedAt, err := ctx.PushedAt("e05fcae367230ee709313dd2720da527d178ce43")
		require.NoError(t, err)

		assert.Equal(t, expectedTime, pushedAt, "incorrect pushed at for commit")
		assert.Equal(t, 3, statusRuleE05.Count, "incorrect http request count")

		cachedPushedAt := gc.PushedAt["1234:e05fcae367230ee709313dd2720da527d178ce43"]
		assert.Equal(t, expectedTime, cachedPushedAt, "incorrect value in global cache")
	})

	t.Run("fromBatch", func(t *testing.T) {
		pushedAt, err := ctx.PushedAt("a6f3f69b64eaafece5a0d854eb4af11c0d64394c")
		require.NoError(t, err)

		assert.Equal(t, expectedTime, pushedAt, "incorrect pushed at for commit")
		assert.Equal(t, 2, commitsRule.Count, "incorrect http request count")
		assert.Equal(t, 1, statusRuleA6F.Count, "incorrect http request count")
		assert.Equal(t, 1, statusRule1FC.Count, "incorrect http request count")
		assert.Equal(t, 3, statusRuleE05.Count, "incorrect http request count")
	})

	t.Run("fromBatchCache", func(t *testing.T) {
		pushedAt, err := ctx.PushedAt("a6f3f69b64eaafece5a0d854eb4af11c0d64394c")
		require.NoError(t, err)

		assert.Equal(t, expectedTime, pushedAt, "incorrect pushed at for commit")
		assert.Equal(t, 2, commitsRule.Count, "incorrect http request count")
		assert.Equal(t, 1, statusRuleA6F.Count, "incorrect http request count")
		assert.Equal(t, 1, statusRule1FC.Count, "incorrect http request count")
		assert.Equal(t, 3, statusRuleE05.Count, "incorrect http request count")
	})

	t.Run("fromGlobalCache", func(t *testing.T) {
		ctx := makeContext(t, rp, nil, gc)

		pushedAt, err := ctx.PushedAt("e05fcae367230ee709313dd2720da527d178ce43")
		require.NoError(t, err)

		assert.Equal(t, expectedTime, pushedAt, "incorrect pushed at for commit")
		assert.Equal(t, 3, statusRuleE05.Count, "incorrect http request count")
	})
}

func makeContext(t *testing.T, rp *ResponsePlayer, pr *github.PullRequest, gc GlobalCache) Context {
	ctx := context.Background()
	client := github.NewClient(&http.Client{Transport: rp})
	v4client := githubv4.NewClient(&http.Client{Transport: rp})

	base, _ := url.Parse("http://github.localhost/")
	client.BaseURL = base

	mbrCtx := NewGitHubMembershipContext(ctx, client)
	if pr == nil {
		pr = defaultTestPR()
	}

	prctx, err := NewGitHubContext(ctx, mbrCtx, gc, client, v4client, Locator{
		Owner:  pr.GetBase().GetRepo().GetOwner().GetLogin(),
		Repo:   pr.GetBase().GetRepo().GetName(),
		Number: pr.GetNumber(),
		Value:  pr,
	})
	require.NoError(t, err, "failed to create github context")

	return prctx
}

func defaultTestPR() *github.PullRequest {
	return &github.PullRequest{
		Title:     github.String("test title"),
		State:     github.String("open"),
		Number:    github.Int(123),
		CreatedAt: &github.Timestamp{Time: time.Date(2020, 9, 30, 17, 42, 10, 0, time.UTC)},
		Draft:     github.Bool(false),
		User: &github.User{
			Login: github.String("mhaypenny"),
		},
		Head: &github.PullRequestBranch{
			Ref: github.String("test-branch"),
			SHA: github.String("e05fcae367230ee709313dd2720da527d178ce43"),
			Repo: &github.Repository{
				ID: github.Int64(1234),
				Owner: &github.User{
					Login: github.String("testorg"),
				},
				Name: github.String("testrepo"),
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("develop"),
			Repo: &github.Repository{
				ID: github.Int64(1234),
				Owner: &github.User{
					Login: github.String("testorg"),
				},
				Name: github.String("testrepo"),
			},
		},
	}
}

func newTime(t time.Time) *time.Time {
	return &t
}

type MockGlobalCache struct {
	PushedAt map[string]time.Time
}

func NewMockGlobalCache() *MockGlobalCache {
	return &MockGlobalCache{
		PushedAt: make(map[string]time.Time),
	}
}

func (c *MockGlobalCache) GetPushedAt(repoID int64, sha string) (time.Time, bool) {
	t, ok := c.PushedAt[fmt.Sprintf("%d:%s", repoID, sha)]
	return t, ok
}

func (c *MockGlobalCache) SetPushedAt(repoID int64, sha string, t time.Time) {
	c.PushedAt[fmt.Sprintf("%d:%s", repoID, sha)] = t
}
