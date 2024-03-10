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

package simulated

import (
	"testing"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestComments(t *testing.T) {
	tests := map[string]struct {
		Comments         []*pull.Comment
		Options          Options
		ExpectedComments []*pull.Comment
		TeamMembership   map[string][]string
		OrgMembership    map[string][]string
		Collaborators    []*pull.Collaborator
		ExpectedError    bool
	}{
		"ignore comments by user": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
			},
			Options: Options{
				IgnoreComments: &Actors{
					Users: []string{"iignore"},
				},
			},
		},
		"ignore comments by team membership": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &Actors{
					Teams: []string{"test-team-1"},
				},
			},
			TeamMembership: map[string][]string{
				"iignore": {"test-team-1"},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
			},
		},
		"ignore comments by org membership": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &Actors{
					Organizations: []string{"test-org-1"},
				},
			},
			OrgMembership: map[string][]string{
				"iignore": {"test-org-1"},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
			},
		},
		"ignore comments by permission": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &Actors{
					Permissions: []string{"read"},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
			},
		},
		"return error when invalid permission used": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &Actors{
					Permissions: []string{"bread"},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedError: true,
		},
		"do not ignore any comments": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
		},
		"add new comment by sperson": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				AddComments: []Comment{
					{Author: "sperson", Body: ":+1:"},
				},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
				{Author: "sperson"},
			},
		},
		"add new comment by sperson and ignore one from iignore": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				AddComments: []Comment{
					{Author: "sperson", Body: ":+1:"},
				},
				IgnoreComments: &Actors{
					Users: []string{"iignore"},
				},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "sperson"},
			},
		},
	}

	for message, test := range tests {
		test.Options.setDefaults()
		context := Context{
			Context: &pulltest.Context{
				CommentsValue:      test.Comments,
				TeamMemberships:    test.TeamMembership,
				OrgMemberships:     test.OrgMembership,
				CollaboratorsValue: test.Collaborators,
			},
			options: test.Options,
		}

		comments, err := context.Comments()
		if test.ExpectedError {
			assert.Error(t, err, message)
		} else {
			assert.NoError(t, err, test, message)
			assert.Equal(t, getCommentAuthors(test.ExpectedComments), getCommentAuthors(comments), message)
		}
	}
}

func getCommentAuthors(comments []*pull.Comment) []string {
	var authors []string
	for _, c := range comments {
		authors = append(authors, c.Author)
	}

	return authors
}

func TestReviews(t *testing.T) {
	tests := map[string]struct {
		Reviews         []*pull.Review
		Options         Options
		ExpectedReviews []*pull.Review
		ExpectedError   bool
		TeamMembership  map[string][]string
		OrgMembership   map[string][]string
		Collaborators   []*pull.Collaborator
	}{
		"ignore reviews by iignore": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
			},
			Options: Options{
				IgnoreReviews: &Actors{
					Users: []string{"iignore"},
				},
			},
		},
		"ignore reviews by team membership": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &Actors{
					Teams: []string{"test-team-1"},
				},
			},
			TeamMembership: map[string][]string{
				"iignore": {"test-team-1"},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
			},
		},
		"ignore reviews by org membership": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &Actors{
					Organizations: []string{"test-org-1"},
				},
			},
			OrgMembership: map[string][]string{
				"iignore": {"test-org-1"},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
			},
		},
		"ignore reviews by permission": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &Actors{
					Permissions: []string{"read"},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
			},
		},
		"return error when invalid permission used": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &Actors{
					Permissions: []string{"bread"},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedError: true,
		},
		"do not ignore any reviews": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
		},
		"add new review by sperson": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				AddReviews: []Review{
					{Author: "sperson", State: "approved"},
				},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
				{Author: "sperson"},
			},
		},
		"add new review by sperson and ignore one from iignore": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				AddReviews: []Review{
					{Author: "sperson", State: "approved"},
				},
				IgnoreReviews: &Actors{
					Users: []string{"iignore"},
				},
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "sperson"},
			},
		},
	}

	for message, test := range tests {
		test.Options.setDefaults()
		context := Context{
			Context: &pulltest.Context{
				ReviewsValue:       test.Reviews,
				TeamMemberships:    test.TeamMembership,
				OrgMemberships:     test.OrgMembership,
				CollaboratorsValue: test.Collaborators,
			},
			options: test.Options,
		}

		reviews, err := context.Reviews()
		if test.ExpectedError {
			assert.Error(t, err, message)
		} else {
			assert.NoError(t, err, test, message)
			assert.Equal(t, getReviewAuthors(test.ExpectedReviews), getReviewAuthors(reviews), message)
		}
	}
}

func getReviewAuthors(reviews []*pull.Review) []string {
	var authors []string
	for _, c := range reviews {
		authors = append(authors, c.Author)
	}

	return authors
}

func TestBranches(t *testing.T) {
	tests := map[string]struct {
		Base         string
		Head         string
		Options      Options
		ExpectedBase string
		ExpectedHead string
	}{
		"use default base branch": {
			Base:         "develop",
			Head:         "aa/feature1",
			ExpectedBase: "develop",
			ExpectedHead: "aa/feature1",
		},
		"use simulated base branch": {
			Base:         "develop",
			Head:         "aa/feature1",
			ExpectedBase: "simulated-develop",
			ExpectedHead: "aa/feature1",
			Options: Options{
				BaseBranch: "simulated-develop",
			},
		},
	}

	for message, test := range tests {
		test.Options.setDefaults()
		context := Context{
			Context: &pulltest.Context{BranchBaseName: test.Base, BranchHeadName: test.Head},
			options: test.Options,
		}

		base, head := context.Branches()
		assert.Equal(t, test.ExpectedBase, base, message)
		assert.Equal(t, test.ExpectedHead, head, message)
	}
}
