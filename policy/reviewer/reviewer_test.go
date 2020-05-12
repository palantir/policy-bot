// Copyright 2019 Palantir Technologies, Inc.
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

package reviewer

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestFindLeafResults(t *testing.T) {
	results := makeResults(&common.Result{
		Name:        "Skipped",
		Description: "",
		Status:      common.StatusSkipped,
		Error:       nil,
		Children:    nil,
	}, "random-users")
	actualResults := findLeafChildren(results)
	require.Len(t, actualResults, 2, "incorrect number of leaf results")
}

func TestSelectRandomUsers(t *testing.T) {
	r := rand.New(rand.NewSource(42))

	require.Len(t, selectRandomUsers(0, []string{"a"}, r), 0, "0 selection should return nothing")
	require.Len(t, selectRandomUsers(1, []string{}, r), 0, "empty list should return nothing")

	assert.Equal(t, []string{"a"}, selectRandomUsers(1, []string{"a"}, r))
	assert.Equal(t, []string{"a", "b"}, selectRandomUsers(3, []string{"a", "b"}, r))

	pseudoRandom := selectRandomUsers(1, []string{"a", "b", "c"}, r)
	assert.Equal(t, []string{"c"}, pseudoRandom)

	multiplePseudoRandom := selectRandomUsers(4, []string{"a", "b", "c", "d", "e", "f", "g"}, r)
	assert.Equal(t, []string{"c", "e", "b", "f"}, multiplePseudoRandom)
}

func TestFindRepositoryCollaborators(t *testing.T) {
	prctx := makeContext()
	collabPerms, err := prctx.RepositoryCollaborators()
	var collabs []string
	for c := range collabPerms {
		collabs = append(collabs, c)
	}
	sort.Strings(collabs)
	require.NoError(t, err)
	require.Equal(t, []string{"contributor-author", "contributor-committer", "mhaypenny", "org-owner", "review-approver", "user-direct-admin", "user-team-admin", "user-team-write"}, collabs)
}

func TestSelectReviewers(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Owner",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			Admins:        true,
			RequiredCount: 1,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()

	reviewers, _, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, reviewers, 3, "policy should request three people")
	require.Contains(t, reviewers, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, reviewers, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, reviewers, "not-a-collaborator", "a non collaborator cannot be requested")
	require.NotContains(t, reviewers, "org-owner", "org-owner should not be requested")
}

func TestSelectAdminTeam(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Owner",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			Admins:        true,
			RequiredCount: 1,
			Mode:          "teams",
		},
		Error:    nil,
		Children: nil,
	}, "teams")

	prctx := makeContext()

	reviewers, teams, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, teams, 1, "admin team should be selected")
	require.Contains(t, teams, "team-admin", "admin team seleted")

	require.Len(t, reviewers, 0, "policy should request no people")
}

func TestSelectReviewers_Team(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Team",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			// Require a team approval
			Teams:         []string{"everyone/team-write"},
			RequiredCount: 1,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()
	reviewers, teams, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Empty(t, teams, "no teams should be returned")
	require.Len(t, reviewers, 3, "policy should request three people")
	require.Contains(t, reviewers, "review-approver", "at least review-approver must be selected")
	require.Contains(t, reviewers, "user-team-write", "at least user-team-write must be selected")
	require.NotContains(t, reviewers, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, reviewers, "not-a-collaborator", "a non collaborator cannot be requested")
}

func TestSelectReviewers_Team_teams(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Team",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			// Require a team approval
			Teams:         []string{"everyone/team-write", "everyone/team-not-collaborators"},
			Users:         []string{"user-team-write"},
			RequiredCount: 1,
			Mode:          "teams",
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()
	reviewers, teams, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, teams, 1, "one team should be returned")
	require.Contains(t, teams, "team-write", "team-write should be selected")
	require.Len(t, reviewers, 2, "policy should request 2 people")
	require.Contains(t, reviewers, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, reviewers, "user-team-write", "user-team-write should not be selected")
	require.NotContains(t, reviewers, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, reviewers, "not-a-collaborator", "a non collaborator cannot be requested")
}

func TestSelectReviewers_Team_teamsDefaultsToNothing(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Team",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			// Require a team approval
			Teams:         []string{"everyone/team-not-collaborators"},
			Users:         []string{"user-team-write"},
			RequiredCount: 1,
			Mode:          "teams",
		},
		Error:    nil,
		Children: nil,
	}, "teams")

	prctx := makeContext()
	reviewers, teams, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Empty(t, teams, "no team should be returned")
	require.Len(t, reviewers, 0, "policy should request no people")
}

func TestSelectReviewers_Org(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:        "Team",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			// Require everyone org approval
			Organizations: []string{"everyone"},
			RequiredCount: 1,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()
	reviewers, _, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, reviewers, 3, "policy should request three people")
	require.Contains(t, reviewers, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, reviewers, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, reviewers, "not-a-collaborator", "a non collaborator cannot be requested")
}

func makeResults(result *common.Result, mode string) common.Result {
	results := common.Result{
		Name:        "One",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			Users:         []string{"neverappears"},
			RequiredCount: 0,
			Mode:          common.RequestMode(mode),
		},
		Error: nil,
		Children: []*common.Result{{
			Name:        "Two",
			Description: "",
			Status:      common.StatusPending,
			ReviewRequestRule: common.ReviewRequestRule{
				Users:         []string{"mhaypenny", "review-approver"},
				RequiredCount: 1,
				Mode:          common.RequestMode(mode),
			},
			Error:    nil,
			Children: nil,
		},
			result,
			{
				Name:              "Three",
				Description:       "",
				Status:            common.StatusDisapproved,
				ReviewRequestRule: common.ReviewRequestRule{},
				Error:             errors.New("foo"),
				Children:          nil,
			},
			{
				Name:              "Four",
				Description:       "",
				Status:            common.StatusPending,
				ReviewRequestRule: common.ReviewRequestRule{},
				Error:             nil,
				Children: []*common.Result{{
					Name:        "Five",
					Description: "",
					Status:      common.StatusPending,
					ReviewRequestRule: common.ReviewRequestRule{
						Users:              []string{"contributor-committer", "contributor-author", "not-a-collaborator"},
						RequiredCount:      1,
						WriteCollaborators: true,
						Admins:             false,
						Mode:               common.RequestMode(mode),
					},
					Error:    nil,
					Children: nil,
				},
				},
			},
		},
	}
	return results
}

func makeContext() pull.Context {
	return &pulltest.Context{
		OwnerValue: "everyone",

		AuthorValue:   "mhaypenny",
		CommentsValue: []*pull.Comment{},
		ReviewsValue:  []*pull.Review{},

		OrgMemberships: map[string][]string{
			"mhaypenny":             {"everyone"},
			"contributor-author":    {"everyone"},
			"contributor-committer": {"everyone"},
			"comment-approver":      {"everyone", "cool-org"},
			"review-approver":       {"everyone", "even-cooler-org"},
		},
		CollaboratorMemberships: map[string][]string{
			"mhaypenny":             {common.GithubAdminPermission},
			"org-owner":             {common.GithubAdminPermission},
			"user-team-admin":       {common.GithubAdminPermission},
			"user-direct-admin":     {common.GithubAdminPermission},
			"user-team-write":       {common.GithubWritePermission},
			"contributor-committer": {common.GithubWritePermission},
			"contributor-author":    {common.GithubWritePermission},
			"review-approver":       {common.GithubWritePermission},
		},
		TeamsValue: map[string]string{
			"team-write": common.GithubWritePermission,
			"team-admin": common.GithubAdminPermission,
		},
		TeamMemberships: map[string][]string{
			"user-team-admin":    {"everyone/team-admin"},
			"user-team-write":    {"everyone/team-write"},
			"not-a-collaborator": {"everyone/team-not-collaborators"},
		},
	}
}
