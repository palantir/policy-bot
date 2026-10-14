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
	"testing"

	"github.com/google/go-github/v90/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestTemplates(t *testing.T) func(name string, data any) string {
	t.Helper()

	tree, err := LoadTemplates(&FilesConfig{
		Templates: "../templates",
		Static:    "../../build/static",
	}, "", "https://github.com")
	require.NoError(t, err, "templates failed to parse")

	return func(name string, data any) string {
		var buf bytes.Buffer
		require.NoError(t, tree.ExecuteTemplate(&buf, name, data), "template failed to render")
		return buf.String()
	}
}

// TestDetailsTemplateRendersDisqualifications renders the details page for a
// rule whose approvals were all ignored, and checks that each user is named
// along with why. See https://github.com/palantir/policy-bot/issues/766.
func TestDetailsTemplateRendersDisqualifications(t *testing.T) {
	render := loadTestTemplates(t)

	pr := &github.PullRequest{
		Number: new(42),
		Base: &github.PullRequestBranch{
			Repo: &github.Repository{
				FullName: new("testorg/testrepo"),
				Owner:    &github.User{Login: new("testorg")},
				Name:     new("testrepo"),
			},
		},
	}

	data := map[string]any{
		"BasePath":    "",
		"User":        "viewer",
		"PullRequest": pr,
		"Result": &common.Result{
			Name:   "policy",
			Status: common.StatusPending,
			Children: []*common.Result{
				{
					Name:    "rule",
					Status:  common.StatusPending,
					Methods: &common.Methods{Comments: []string{":+1:"}},
					Requires: common.RequiresResult{
						Count:  1,
						Actors: common.Actors{Users: []string{"someone"}},
						Disqualifications: []*common.Disqualification{
							{
								Candidate: &common.Candidate{User: "mhaypenny"},
								Reason:    common.DisqualifiedAuthor,
							},
							{
								Candidate: &common.Candidate{User: "contributor-author"},
								Reason:    common.DisqualifiedContributor,
								Commit:    "674832587eaaf416371b30f5bc5a47e377f534ec",
							},
							{
								Candidate: &common.Candidate{User: "outsider"},
								Reason:    common.DisqualifiedNotRequired,
							},
						},
					},
				},
			},
		},
	}

	out := render("details.html.tmpl", data)

	assert.Contains(t, out, "These approvals were not counted")
	assert.Contains(t, out, "mhaypenny")
	assert.Contains(t, out, "Opened this pull request")
	assert.Contains(t, out, "outsider")
	assert.Contains(t, out, "Not one of the users this rule requires")

	// the contributor's commit is linked, which is what the issue asked for:
	// otherwise the only way to find it is to read every commit
	assert.Contains(t, out, "contributor-author")
	assert.Contains(t, out, "https://github.com/testorg/testrepo/commit/674832587eaaf416371b30f5bc5a47e377f534ec")
	assert.Contains(t, out, "6748325", "the commit is shown abbreviated")
}

// TestDetailsTemplateOmitsEmptyDisqualifications checks the section disappears
// when every approval counted.
func TestDetailsTemplateOmitsEmptyDisqualifications(t *testing.T) {
	render := loadTestTemplates(t)

	pr := &github.PullRequest{
		Number: new(42),
		Base: &github.PullRequestBranch{
			Repo: &github.Repository{
				FullName: new("testorg/testrepo"),
				Owner:    &github.User{Login: new("testorg")},
				Name:     new("testrepo"),
			},
		},
	}

	data := map[string]any{
		"BasePath":    "",
		"User":        "viewer",
		"PullRequest": pr,
		"Result": &common.Result{
			Name:   "policy",
			Status: common.StatusApproved,
			Children: []*common.Result{
				{
					Name:    "rule",
					Status:  common.StatusApproved,
					Methods: &common.Methods{Comments: []string{":+1:"}},
					Requires: common.RequiresResult{
						Count:     1,
						Actors:    common.Actors{Users: []string{"someone"}},
						Approvers: []*common.Candidate{{User: "someone"}},
					},
				},
			},
		},
	}

	out := render("details.html.tmpl", data)
	assert.NotContains(t, out, "These approvals were not counted")
}
