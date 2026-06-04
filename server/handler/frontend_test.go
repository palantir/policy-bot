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
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailsTemplateShowsAllOwnersForSharedCodeownerGroup(t *testing.T) {
	tmpl, err := LoadTemplates(&FilesConfig{
		Static:    "../../build/static",
		Templates: "../templates",
	}, "", "https://github.com")
	require.NoError(t, err)

	githubReview := true
	data := struct {
		BasePath  string
		User      string
		PolicyURL string

		ExpandRequiredReviewers bool

		Error            error
		IsTemporaryError bool

		PullRequest *github.PullRequest
		Result      *common.Result
		Codeowners  *pull.CodeownersResult
		PullContext pull.Context
	}{
		User:      "designer",
		PolicyURL: "https://github.com/acme/example-repo/blob/main/.policy.yml",
		PullRequest: &github.PullRequest{
			Number:  github.Ptr(248),
			Title:   github.Ptr("Add health check endpoint"),
			HTMLURL: github.Ptr("https://github.com/acme/example-repo/pull/248"),
			Base: &github.PullRequestBranch{
				Ref: github.Ptr("main"),
				Repo: &github.Repository{
					FullName: github.Ptr("acme/example-repo"),
				},
			},
		},
		Result: &common.Result{
			Name:              "approval",
			Status:            common.StatusApproved,
			StatusDescription: "All rules are approved",
			Children: []*common.Result{
				{
					Name:              "shared codeowners",
					Description:       "Owners of shared files must approve",
					Status:            common.StatusApproved,
					StatusDescription: "Approved by dave",
					Methods: &common.Methods{
						GithubReview:      &githubReview,
						GithubReviewState: pull.ReviewApproved,
					},
					Requires: common.RequiresResult{
						Count:  1,
						Actors: common.Actors{Codeowners: true},
						Approvers: []*common.Candidate{
							{Author: &pull.Author{Login: "dave", AvatarURL: "https://avatars.example/dave.png"}},
						},
						OwnershipGroups: []common.OwnershipGroupResult{
							{
								Key:       "@acme/team-alpha,@acme/team-beta",
								Owners:    []string{"@acme/team-alpha", "@acme/team-beta"},
								Files:     []string{"shared/config.yaml"},
								Satisfied: true,
								Approvers: []string{"dave"},
							},
						},
					},
				},
			},
		},
		Codeowners: &pull.CodeownersResult{
			Owners: map[string][]string{
				"shared/config.yaml": {"@acme/team-alpha", "@acme/team-beta"},
			},
		},
	}

	var out bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&out, "details.html.tmpl", data))
	html := out.String()

	assert.Contains(t, html, `title="acme/team-alpha"`)
	assert.Contains(t, html, `title="acme/team-beta"`)
	assert.Contains(t, html, `title="dave"`)

	groupStart := strings.Index(html, `<div class="ownership-group">`)
	require.NotEqual(t, -1, groupStart)
	ownershipGroup := html[groupStart:]
	groupEnd := strings.Index(ownershipGroup, `<details`)
	require.NotEqual(t, -1, groupEnd)
	ownershipGroup = ownershipGroup[:groupEnd]
	assert.Contains(t, ownershipGroup, `title="acme/team-alpha"`)
	assert.Contains(t, ownershipGroup, `title="acme/team-beta"`)
	assert.Contains(t, ownershipGroup, `title="dave"`)
}
