// Copyright 2026 Palantir Technologies, Inc.
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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
)

func TestHasPostGithubReviewRule(t *testing.T) {
	tests := map[string]struct {
		Result   *common.Result
		Expected bool
	}{
		"noPostGithubReview": {
			Result: &common.Result{
				Status: common.StatusApproved,
			},
			Expected: false,
		},
		"postGithubReviewAndApproved": {
			Result: &common.Result{
				PostGithubReview: true,
				Status:           common.StatusApproved,
			},
			Expected: true,
		},
		"postGithubReviewButPending": {
			Result: &common.Result{
				PostGithubReview: true,
				Status:           common.StatusPending,
			},
			Expected: false,
		},
		"postGithubReviewButSkipped": {
			Result: &common.Result{
				PostGithubReview: true,
				Status:           common.StatusSkipped,
			},
			Expected: false,
		},
		"childHasPostGithubReviewAndApproved": {
			Result: &common.Result{
				Status: common.StatusApproved,
				Children: []*common.Result{
					{
						Status: common.StatusApproved,
					},
					{
						PostGithubReview: true,
						Status:           common.StatusApproved,
					},
				},
			},
			Expected: true,
		},
		"childHasPostGithubReviewButPending": {
			Result: &common.Result{
				Status: common.StatusPending,
				Children: []*common.Result{
					{
						PostGithubReview: true,
						Status:           common.StatusPending,
					},
				},
			},
			Expected: false,
		},
		"deeplyNestedPostGithubReview": {
			Result: &common.Result{
				Status: common.StatusApproved,
				Children: []*common.Result{
					{
						Status: common.StatusApproved,
						Children: []*common.Result{
							{
								PostGithubReview: true,
								Status:           common.StatusApproved,
							},
						},
					},
				},
			},
			Expected: true,
		},
		"multipleChildrenOnlyOnePostGithubReview": {
			Result: &common.Result{
				Status: common.StatusApproved,
				Children: []*common.Result{
					{
						Status: common.StatusSkipped,
					},
					{
						Status: common.StatusApproved,
					},
					{
						PostGithubReview: true,
						Status:           common.StatusApproved,
					},
				},
			},
			Expected: true,
		},
		"noChildren": {
			Result:   &common.Result{},
			Expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := hasPostGithubReviewRule(test.Result)
			assert.Equal(t, test.Expected, actual)
		})
	}
}
