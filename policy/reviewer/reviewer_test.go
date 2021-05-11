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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindLeafResults(t *testing.T) {
	result := &common.Result{
		Name:   "Parent",
		Status: common.StatusPending,
		Children: []*common.Result{
			{
				Name:              "One",
				Status:            common.StatusPending,
				ReviewRequestRule: &common.ReviewRequestRule{},
			},
			{
				Name:              "Skipped",
				Status:            common.StatusSkipped,
				ReviewRequestRule: &common.ReviewRequestRule{},
			},
			{
				Name:              "Error",
				Status:            common.StatusPending,
				Error:             errors.New("failed"),
				ReviewRequestRule: &common.ReviewRequestRule{},
			},
			{
				Name:              "Disapproved",
				Status:            common.StatusDisapproved,
				ReviewRequestRule: &common.ReviewRequestRule{},
			},
			{
				Name:   "Two",
				Status: common.StatusPending,
				Children: []*common.Result{
					{
						Name:              "Three",
						Status:            common.StatusPending,
						ReviewRequestRule: &common.ReviewRequestRule{},
					},
				},
			},
		},
	}
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
	results := []*common.Result{
		{
			Name:   "users",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Users:         []string{"mhaypenny", "review-approver", "contributor-committer"},
				RequiredCount: 2,
				Mode:          common.RequestModeRandomUsers,
			},
		},
		{
			Name:   "admin-users",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Permissions:   []pull.Permission{pull.PermissionAdmin},
				RequiredCount: 1,
				Mode:          common.RequestModeRandomUsers,
			},
		},
	}

	prctx := makeContext()

	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Users, 3, "policy should request three people")
	require.Contains(t, selection.Users, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, selection.Users, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, selection.Users, "not-a-collaborator", "a non collaborator cannot be requested")
	require.NotContains(t, selection.Users, "org-owner", "org-owner should not be requested")
}

func TestSelectReviewers_UserPermission(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:   "user-permissions",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Permissions:   []pull.Permission{pull.PermissionTriage, pull.PermissionMaintain},
				RequiredCount: 2,
				Mode:          common.RequestModeAllUsers,
			},
		},
	}

	prctx := makeContext()

	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Teams, 0, "policy should request no teams")

	require.Len(t, selection.Users, 4, "policy should request two users")
	require.Contains(t, selection.Users, "maintainer", "maintainer selected")
	require.Contains(t, selection.Users, "triager", "triager selected")
	require.Contains(t, selection.Users, "org-owner-team-maintainer", "triager selected")
	require.Contains(t, selection.Users, "direct-write-team-maintainer", "triager selected")
}

func TestSelectReviewers_TeamPermission(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:   "team-permissions",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Permissions:   []pull.Permission{pull.PermissionAdmin, pull.PermissionMaintain},
				RequiredCount: 1,
				Mode:          common.RequestModeTeams,
			},
		},
	}

	prctx := makeContext()

	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Teams, 2, "policy should request two teams")
	require.Contains(t, selection.Teams, "team-admin", "admin team seleted")
	require.Contains(t, selection.Teams, "team-maintain", "maintainer team seleted")

	require.Len(t, selection.Users, 0, "policy should request no people")
}

func TestSelectReviewers_TeamMembers(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:   "team-users",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Teams:         []string{"everyone/team-write"},
				RequiredCount: 1,
				Mode:          common.RequestModeRandomUsers,
			},
		},
	}

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Empty(t, selection.Teams, "no teams should be returned")
	require.Len(t, selection.Users, 1, "policy should request one reviewer")
	require.Contains(t, selection.Users, "user-team-write", "user-team-write must be selected")
}

func TestSelectReviewers_Team(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:   "Team",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Teams:         []string{"everyone/team-write", "everyone/team-not-collaborators"},
				Users:         []string{"user-team-write"},
				RequiredCount: 1,
				Mode:          common.RequestModeTeams,
			},
		},
	}

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Teams, 1, "one team should be returned")
	require.Contains(t, selection.Teams, "team-write", "team-write should be selected")
	require.Len(t, selection.Users, 0, "policy should request 0 users")
}

func TestSelectReviewers_TeamNotCollaborator(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := []*common.Result{
		{
			Name:   "not-collaborators",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Teams:         []string{"everyone/team-not-collaborators"},
				Users:         []string{"user-team-write"},
				RequiredCount: 1,
				Mode:          common.RequestModeTeams,
			},
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
	results := []*common.Result{
		{
			Name:   "org",
			Status: common.StatusPending,
			ReviewRequestRule: &common.ReviewRequestRule{
				Organizations: []string{"everyone"},
				RequiredCount: 1,
				Mode:          common.RequestModeRandomUsers,
			},
		},
	}

	prctx := makeContext()
	selection, err := SelectReviewers(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, selection.Users, 1, "policy should request one person")
	require.Contains(t, selection.Users, "review-approver", "review-approver must be selected")
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
			{
				Name: "mhaypenny",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin},
				},
			},
			{
				Name: "org-owner",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin},
				},
			},
			{
				Name: "user-team-admin",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin, ViaRepo: true},
				},
			},
			{
				Name: "user-direct-admin",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin, ViaRepo: true},
				},
			},
			{
				Name: "user-team-write",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionWrite, ViaRepo: true},
				},
			},
			{
				Name: "contributor-committer",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionWrite},
				},
			},
			{
				Name: "contributor-author",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionWrite},
				},
			},
			{
				Name: "review-approver",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionWrite},
				},
			},
			{
				Name: "maintainer",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionMaintain, ViaRepo: true},
				},
			},
			{
				// note: currently not possible in GitHub
				Name: "indirect-maintainer",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionMaintain},
				},
			},
			{
				Name: "triager",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionTriage, ViaRepo: true},
				},
			},
			{
				// note: currently not possible in GitHub
				Name: "indirect-triager",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionTriage},
				},
			},
			{
				Name: "org-owner-team-maintainer",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin, ViaRepo: false},
					{Permission: pull.PermissionMaintain, ViaRepo: true},
				},
			},
			{
				Name: "direct-write-team-maintainer",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionMaintain, ViaRepo: true},
					{Permission: pull.PermissionWrite, ViaRepo: true},
				},
			},
		},
		TeamsValue: map[string]pull.Permission{
			"team-write":    pull.PermissionWrite,
			"team-admin":    pull.PermissionAdmin,
			"team-maintain": pull.PermissionMaintain,
		},
		TeamMemberships: map[string][]string{
			"user-team-admin":    {"everyone/team-admin"},
			"user-team-write":    {"everyone/team-write"},
			"maintainer":         {"everyone/team-maintain"},
			"not-a-collaborator": {"everyone/team-not-collaborators"},
		},
	}
}
