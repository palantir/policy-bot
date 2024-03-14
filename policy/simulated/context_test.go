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
	"sort"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComments(t *testing.T) {
	tests := map[string]struct {
		Comments               []*pull.Comment
		Options                Options
		ExpectedCommentAuthors []string
		TeamMembership         map[string][]string
		OrgMembership          map[string][]string
		Collaborators          []*pull.Collaborator
		ExpectedError          bool
	}{
		"ignore comments by user": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedCommentAuthors: []string{"rrandom"},
			Options: Options{
				IgnoreComments: &common.Actors{
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
				IgnoreComments: &common.Actors{
					Teams: []string{"test-team-1"},
				},
			},
			TeamMembership: map[string][]string{
				"iignore": {"test-team-1"},
			},
			ExpectedCommentAuthors: []string{"rrandom"},
		},
		"ignore comments by org membership": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &common.Actors{
					Organizations: []string{"test-org-1"},
				},
			},
			OrgMembership: map[string][]string{
				"iignore": {"test-org-1"},
			},
			ExpectedCommentAuthors: []string{"rrandom"},
		},
		"ignore comments by permission": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreComments: &common.Actors{
					Permissions: []pull.Permission{pull.PermissionRead},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedCommentAuthors: []string{"rrandom"},
		},
		"do not ignore any comments": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedCommentAuthors: []string{"rrandom", "iignore"},
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
			ExpectedCommentAuthors: []string{"rrandom", "iignore", "sperson"},
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
				IgnoreComments: &common.Actors{
					Users: []string{"iignore"},
				},
			},
			ExpectedCommentAuthors: []string{"rrandom", "sperson"},
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

		sort.Strings(test.ExpectedCommentAuthors)

		comments, err := context.Comments()
		if test.ExpectedError {
			assert.Error(t, err, message)
		} else {
			require.NoError(t, err, message)
			assert.Equal(t, test.ExpectedCommentAuthors, getCommentAuthors(comments), message)
		}
	}
}

func getCommentAuthors(comments []*pull.Comment) []string {
	var authors []string
	for _, c := range comments {
		authors = append(authors, c.Author)
	}

	sort.Strings(authors)
	return authors
}

func TestReviews(t *testing.T) {
	tests := map[string]struct {
		Reviews               []*pull.Review
		Options               Options
		ExpectedReviewAuthors []string
		ExpectedError         bool
		TeamMembership        map[string][]string
		OrgMembership         map[string][]string
		Collaborators         []*pull.Collaborator
	}{
		"ignore reviews by iignore": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedReviewAuthors: []string{"rrandom"},
			Options: Options{
				IgnoreReviews: &common.Actors{
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
				IgnoreReviews: &common.Actors{
					Teams: []string{"test-team-1"},
				},
			},
			TeamMembership: map[string][]string{
				"iignore": {"test-team-1"},
			},
			ExpectedReviewAuthors: []string{"rrandom"},
		},
		"ignore reviews by org membership": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &common.Actors{
					Organizations: []string{"test-org-1"},
				},
			},
			OrgMembership: map[string][]string{
				"iignore": {"test-org-1"},
			},
			ExpectedReviewAuthors: []string{"rrandom"},
		},
		"ignore reviews by permission": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			Options: Options{
				IgnoreReviews: &common.Actors{
					Permissions: []pull.Permission{pull.PermissionRead},
				},
			},
			Collaborators: []*pull.Collaborator{
				{Name: "iignore", Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionRead},
				}},
			},
			ExpectedReviewAuthors: []string{"rrandom"},
		},
		"do not ignore any reviews": {
			Reviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedReviewAuthors: []string{"rrandom", "iignore"},
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
			ExpectedReviewAuthors: []string{"rrandom", "iignore", "sperson"},
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
				IgnoreReviews: &common.Actors{
					Users: []string{"iignore"},
				},
			},
			ExpectedReviewAuthors: []string{"rrandom", "sperson"},
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

		sort.Strings(test.ExpectedReviewAuthors)

		reviews, err := context.Reviews()
		if test.ExpectedError {
			assert.Error(t, err, message)
		} else {
			require.NoError(t, err, message)
			assert.Equal(t, test.ExpectedReviewAuthors, getReviewAuthors(reviews), message)
		}
	}
}

func getReviewAuthors(reviews []*pull.Review) []string {
	var authors []string
	for _, c := range reviews {
		authors = append(authors, c.Author)
	}

	sort.Strings(authors)
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
