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
	results := makeResults()
	actualResults := findLeafChildren(results)
	require.Len(t, actualResults, 3, "incorrect number of leaf results")
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

func TestFindRandomRequesters(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	results := makeResults()

	prctx := makeContext()

	collabPerms, err := prctx.RepositoryCollaborators()
	var collabs []string
	for c := range collabPerms {
		collabs = append(collabs, c)
	}
	sort.Strings(collabs)
	require.NoError(t, err)
	require.Equal(t, []string{"contributor-author", "contributor-committer", "mhaypenny", "org-owner", "review-approver"}, collabs)

	reviewers, err := FindRandomRequesters(context.Background(), prctx, results, r)
	require.NoError(t, err)
	require.Len(t, reviewers, 2, "policy should request two people")
	require.Contains(t, reviewers, "review-approver", "at least review-approver must be selected")
	require.NotContains(t, reviewers, "mhaypenny", "the author cannot be requested")
	require.NotContains(t, reviewers, "not-a-collaborator", "a non collaborator cannot be requested")
	require.NotContains(t, reviewers, "org-owner", "org-owner should not be requested")
}

func makeResults() common.Result {
	results := common.Result{
		Name:        "One",
		Description: "",
		Status:      common.StatusPending,
		ReviewRequestRule: common.ReviewRequestRule{
			Users:         []string{"neverappears"},
			RequiredCount: 0,
		},
		Error: nil,
		Children: []*common.Result{{
			Name:        "Two",
			Description: "",
			Status:      common.StatusPending,
			ReviewRequestRule: common.ReviewRequestRule{
				Users:         []string{"mhaypenny", "review-approver"},
				RequiredCount: 1,
			},
			Error:    nil,
			Children: nil,
		},
			{
				Name:              "Three",
				Description:       "",
				Status:            common.StatusDisapproved,
				ReviewRequestRule: common.ReviewRequestRule{},
				Error:             errors.New("foo"),
				Children:          nil,
			},
			{
				Name:        "Owner",
				Description: "",
				Status:      common.StatusPending,
				ReviewRequestRule: common.ReviewRequestRule{
					OrgOwners:     false,
					Admins:        false,
					Users:         []string{"org-owner"},
					RequiredCount: 1,
				},
				Error:    nil,
				Children: nil,
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
						Admins:             true,
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
		AuthorValue:   "mhaypenny",
		CommentsValue: []*pull.Comment{},
		ReviewsValue:  []*pull.Review{},

		OrgOwners: []string{
			"org-owner",
		},

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
			"contributor-committer": {common.GithubAdminPermission},
			"contributor-author":    {common.GithubWritePermission},
			"review-approver":       {common.GithubWritePermission},
		},
	}
}
