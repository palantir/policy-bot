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
	"github.com/stretchr/testify/assert"
)

func TestComments(t *testing.T) {
	tests := map[string]struct {
		Comments         []*pull.Comment
		Options          Options
		ExpectedComments []*pull.Comment
	}{
		"ignore comments by iignore": {
			Comments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "iignore"},
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
			},
			Options: Options{
				Ignore: "iignore",
			},
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
				AddApprovalComment: "sperson",
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
				Ignore:             "iignore",
				AddApprovalComment: "sperson",
			},
			ExpectedComments: []*pull.Comment{
				{Author: "rrandom"},
				{Author: "sperson"},
			},
		},
	}

	for message, test := range tests {
		context := Context{
			Context: &testPullContext{comments: test.Comments},
			options: test.Options,
		}

		comments, err := context.Comments()
		assert.NoError(t, err, test, message)
		assert.Equal(t, commentAuthors(test.ExpectedComments), commentAuthors(comments), message)
	}
}

func TestReviews(t *testing.T) {
	tests := map[string]struct {
		Reviews         []*pull.Review
		Options         Options
		ExpectedReviews []*pull.Review
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
				Ignore: "iignore",
			},
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
				AddApprovalReview: "sperson",
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
				Ignore:            "iignore",
				AddApprovalReview: "sperson",
			},
			ExpectedReviews: []*pull.Review{
				{Author: "rrandom"},
				{Author: "sperson"},
			},
		},
	}

	for message, test := range tests {
		context := Context{
			Context: &testPullContext{reviews: test.Reviews},
			options: test.Options,
		}

		reviews, err := context.Reviews()
		assert.NoError(t, err, test, message)
		assert.Equal(t, reviewAuthors(test.ExpectedReviews), reviewAuthors(reviews), message)
	}
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
		context := Context{
			Context: &testPullContext{base: test.Base, head: test.Head},
			options: test.Options,
		}

		base, head := context.Branches()
		assert.Equal(t, test.ExpectedBase, base, message)
		assert.Equal(t, test.ExpectedHead, head, message)
	}
}

func commentAuthors(comments []*pull.Comment) []string {
	var authors []string
	for _, c := range comments {
		authors = append(authors, c.Author)
	}

	return authors
}

func reviewAuthors(reviews []*pull.Review) []string {
	var authors []string
	for _, c := range reviews {
		authors = append(authors, c.Author)
	}

	return authors
}

type testPullContext struct {
	pull.Context
	comments []*pull.Comment
	reviews  []*pull.Review
	base     string
	head     string
}

func (c *testPullContext) Comments() ([]*pull.Comment, error) {
	return c.comments, nil
}

func (c *testPullContext) Reviews() ([]*pull.Review, error) {
	return c.reviews, nil
}

func (c *testPullContext) Branches() (string, string) {
	return c.base, c.head
}
