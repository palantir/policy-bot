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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestFindLeafResults(t *testing.T) {
	result := makeResult(&common.Result{
		Name:              "Skipped",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusSkipped,
		Error:             nil,
		Children:          nil,
	}, "random-users")
	actualResults := FindRequests(result)
	require.Len(t, actualResults, 2, "incorrect number of leaf results")
}

func TestSelectionDifference(t *testing.T) {
	tests := map[string]struct {
		Input     Selection
		Reviewers []*pull.Reviewer
		Output    Selection
	}{
		"users": {
			Input: Selection{
				Users: []string{"a", "b", "c"},
			},
			Reviewers: []*pull.Reviewer{
				{
					Type: pull.ReviewerUser,
					Name: "a",
				},
				{
					Type: pull.ReviewerUser,
					Name: "c",
				},
				{
					Type: pull.ReviewerTeam,
					Name: "team-a",
				},
			},
			Output: Selection{
				Users: []string{"b"},
			},
		},
		"teams": {
			Input: Selection{
				Teams: []string{"team-a", "team-b", "team-c"},
			},
			Reviewers: []*pull.Reviewer{
				{
					Type: pull.ReviewerUser,
					Name: "a",
				},
				{
					Type: pull.ReviewerUser,
					Name: "c",
				},
				{
					Type: pull.ReviewerTeam,
					Name: "team-a",
				},
			},
			Output: Selection{
				Teams: []string{"team-b", "team-c"},
			},
		},
		"dismissedUsers": {
			Input: Selection{
				Users: []string{"a", "b", "c"},
			},
			Reviewers: []*pull.Reviewer{
				{
					Type:    pull.ReviewerUser,
					Name:    "a",
					Removed: true,
				},
				{
					Type: pull.ReviewerUser,
					Name: "c",
				},
			},
			Output: Selection{
				Users: []string{"b"},
			},
		},
		"dismissedTeams": {
			Input: Selection{
				Teams: []string{"team-a", "team-b", "team-c"},
			},
			Reviewers: []*pull.Reviewer{
				{
					Type: pull.ReviewerTeam,
					Name: "team-a",
				},
				{
					Type:    pull.ReviewerTeam,
					Name:    "team-c",
					Removed: true,
				},
			},
			Output: Selection{
				Teams: []string{"team-b"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			out := test.Input.Difference(test.Reviewers)
			assert.Equal(t, test.Output.Users, out.Users, "incorrect users in difference")
			assert.Equal(t, test.Output.Teams, out.Teams, "incorrect users in difference")
		})
	}
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

func TestSelectReviewers(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:              "Owner",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusPending,
		ReviewRequestRule: &common.ReviewRequestRule{
			Permissions:   []pull.Permission{pull.PermissionAdmin},
			RequiredCount: 1,
			Mode:          common.RequestModeRandomUsers,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()

	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Users, 3, "policy should request three people")
	require.Contains(t, selection.Users, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, selection.Users, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, selection.Users, "not-a-collaborator", "a non collaborator cannot be requested")
	require.NotContains(t, selection.Users, "org-owner", "org-owner should not be requested")
}

func TestSelectAdminTeam(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:              "Owner",
			Description:       "",
			StatusDescription: "",
			Status:            common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Permissions:   []pull.Permission{pull.PermissionAdmin},
				RequiredCount: 1,
				Mode:          "teams",
			},
			Error:    nil,
			Children: nil,
		},
	}

	prctx := makeContext()

	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Teams, 1, "admin team should be selected")
	require.Contains(t, selection.Teams, "team-admin", "admin team seleted")

	require.Len(t, selection.Users, 0, "policy should request no people")
}

func TestSelectReviewers_Team(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:              "Team",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusPending,
		ReviewRequestRule: &common.ReviewRequestRule{
			// Require a team approval
			Teams:         []string{"everyone/team-write"},
			RequiredCount: 1,
			Mode:          common.RequestModeRandomUsers,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Empty(t, selection.Teams, "no teams should be returned")
	require.Len(t, selection.Users, 3, "policy should request three people")
	require.Contains(t, selection.Users, "review-approver", "at least review-approver must be selected")
	require.Contains(t, selection.Users, "user-team-write", "at least user-team-write must be selected")
	require.NotContains(t, selection.Users, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, selection.Users, "not-a-collaborator", "a non collaborator cannot be requested")
}

func TestSelectReviewers_Team_teams(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:              "Team",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusPending,
		ReviewRequestRule: &common.ReviewRequestRule{
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
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Teams, 1, "one team should be returned")
	require.Contains(t, selection.Teams, "team-write", "team-write should be selected")
	require.Len(t, selection.Users, 2, "policy should request 2 people")
	require.Contains(t, selection.Users, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, selection.Users, "user-team-write", "user-team-write should not be selected")
	require.NotContains(t, selection.Users, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, selection.Users, "not-a-collaborator", "a non collaborator cannot be requested")
}

func TestSelectReviewers_Team_teamsDefaultsToNothing(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:              "Team",
			Description:       "",
			StatusDescription: "",
			Status:            common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				// Require a team approval
				Teams:         []string{"everyone/team-not-collaborators"},
				Users:         []string{"user-team-write"},
				RequiredCount: 1,
				Mode:          "teams",
			},
			Error:    nil,
			Children: nil,
		},
	}

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Empty(t, selection.Teams, "no team should be returned")
	require.Len(t, selection.Users, 0, "policy should request no people")
}

func TestSelectReviewers_Org(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults(&common.Result{
		Name:              "Team",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusPending,
		ReviewRequestRule: &common.ReviewRequestRule{
			// Require everyone org approval
			Organizations: []string{"everyone"},
			RequiredCount: 1,
			Mode:          common.RequestModeRandomUsers,
		},
		Error:    nil,
		Children: nil,
	}, "random-users")

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Users, 3, "policy should request three people")
	require.Contains(t, selection.Users, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, selection.Users, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, selection.Users, "not-a-collaborator", "a non collaborator cannot be requested")
}

func makeResults(result *common.Result, mode string) []*common.Result {
	return FindRequests(makeResult(result, mode))
}

func makeResult(result *common.Result, mode string) *common.Result {
	return &common.Result{
		Name:              "One",
		Description:       "",
		StatusDescription: "",
		Status:            common.StatusPending,
		ReviewRequestRule: &common.ReviewRequestRule{
			Users:         []string{"neverappears"},
			RequiredCount: 0,
			Mode:          common.RequestMode(mode),
		},
		Error: nil,
		Children: []*common.Result{
			{
				Name:              "Two",
				Description:       "",
				StatusDescription: "",
				Status:            common.StatusPending,
				ReviewRequestRule: &common.ReviewRequestRule{
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
				StatusDescription: "",
				Status:            common.StatusDisapproved,
				ReviewRequestRule: &common.ReviewRequestRule{},
				Error:             errors.New("foo"),
				Children:          nil,
			},
			{
				Name:              "Four",
				Description:       "",
				StatusDescription: "",
				Status:            common.StatusPending,
				ReviewRequestRule: &common.ReviewRequestRule{},
				Error:             nil,
				Children: []*common.Result{
					{
						Name:              "Five",
						Description:       "",
						StatusDescription: "",
						Status:            common.StatusPending,
						ReviewRequestRule: &common.ReviewRequestRule{
							Users:         []string{"contributor-committer", "contributor-author", "not-a-collaborator"},
							RequiredCount: 1,
							Permissions:   []pull.Permission{pull.PermissionWrite},
							Mode:          common.RequestMode(mode),
						},
						Error:    nil,
						Children: nil,
					},
				},
			},
		},
	}
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
		CollaboratorsValue: []*pull.Collaborator{
			{Name: "mhaypenny", Permission: pull.PermissionAdmin},
			{Name: "org-owner", Permission: pull.PermissionAdmin},
			{Name: "user-team-admin", Permission: pull.PermissionAdmin},
			{Name: "user-direct-admin", Permission: pull.PermissionAdmin},
			{Name: "user-team-write", Permission: pull.PermissionWrite},
			{Name: "contributor-committer", Permission: pull.PermissionWrite},
			{Name: "contributor-author", Permission: pull.PermissionWrite},
			{Name: "review-approver", Permission: pull.PermissionWrite},
		},
		TeamsValue: map[string]pull.Permission{
			"team-write": pull.PermissionWrite,
			"team-admin": pull.PermissionAdmin,
		},
		TeamMemberships: map[string][]string{
			"user-team-admin":    {"everyone/team-admin"},
			"user-team-write":    {"everyone/team-write"},
			"not-a-collaborator": {"everyone/team-not-collaborators"},
		},
	}
}
