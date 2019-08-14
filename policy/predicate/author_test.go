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

package predicate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestHasAuthorIn(t *testing.T) {
	p := &HasAuthorIn{
		common.Actors{
			Teams:         []string{"testorg/team"},
			Users:         []string{"mhaypenny"},
			Organizations: []string{"testorg"},
		},
	}

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"noMatch",
			false,
			&pulltest.Context{
				AuthorValue: "ttest",
				TeamMemberships: map[string][]string{
					"ttest": {
						"boringorg/testers",
					},
				},
				OrgMemberships: map[string][]string{
					"ttest": {
						"boringorg",
					},
				},
			},
		},
		{
			"authorInUsers",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
			},
		},
		{
			"authorInTeams",
			true,
			&pulltest.Context{
				AuthorValue: "mortonh",
				TeamMemberships: map[string][]string{
					"mortonh": {
						"coolorg/approvers",
						"testorg/team",
					},
				},
			},
		},
		{
			"authorInOrgs",
			true,
			&pulltest.Context{
				AuthorValue: "mortonh",
				OrgMemberships: map[string][]string{
					"mortonh": {
						"coolorg",
						"testorg",
					},
				},
			},
		},
	})
}

func TestOnlyHasAuthorsIn(t *testing.T) {
	p := &OnlyHasAuthorsIn{
		common.Actors{
			Teams:         []string{"testorg/team"},
			Users:         []string{"mhaypenny"},
			Organizations: []string{"testorg"},
		},
	}

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"noMatch",
			false,
			&pulltest.Context{
				AuthorValue: "ttest",
				TeamMemberships: map[string][]string{
					"ttest": {
						"boringorg/testers",
					},
				},
				OrgMemberships: map[string][]string{
					"ttest": {
						"boringorg",
					},
				},
			},
		},
		{
			"authorInUsers",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
			},
		},
		{
			"authorInTeams",
			true,
			&pulltest.Context{
				AuthorValue: "mortonh",
				TeamMemberships: map[string][]string{
					"mortonh": {
						"coolorg/approvers",
						"testorg/team",
					},
				},
			},
		},
		{
			"authorInOrgs",
			true,
			&pulltest.Context{
				AuthorValue: "mortonh",
				OrgMemberships: map[string][]string{
					"mortonh": {
						"coolorg",
						"testorg",
					},
				},
			},
		},
		{
			"someAuthorsNotInUsers",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{Author: "notmhaypenny"},
				},
			},
		},
		{
			"prAuthorNotInUsers",
			false,
			&pulltest.Context{
				AuthorValue: "notmhaypenny",
				CommitsValue: []*pull.Commit{
					{Author: "mhaypenny"},
				},
			},
		},
	})
}

func TestHasContributorIn(t *testing.T) {
	p := &HasContributorIn{
		common.Actors{
			Teams:         []string{"testorg/team"},
			Users:         []string{"mhaypenny"},
			Organizations: []string{"testorg"},
		},
	}

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"commitAuthorInUsers",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"commitCommitterInUsers",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"commitAuthorInTeam",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				TeamMemberships: map[string][]string{
					"mhaypenny": {
						"testorg/team",
					},
				},
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"commitAuthorInOrg",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				OrgMemberships: map[string][]string{
					"mhaypenny": {
						"testorg",
					},
				},
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
		},
	})
}

func TestAuthorIsOnlyContributor(t *testing.T) {
	p := AuthorIsOnlyContributor(true)

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"authorIsOnlyContributor",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"authorIsOnlyContributorViaWeb",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:             "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:          "mhaypenny",
						Committer:       "",
						CommittedViaWeb: true,
					},
				},
			},
		},
		{
			"authorIsNotOnlyAuthor",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "ttest",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"authorIsNotOnlyCommitter",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "mhaypenny",
						Committer: "ttest",
					},
				},
			},
		},
	})
}

func TestAuthorIsNotOnlyContributor(t *testing.T) {
	p := AuthorIsOnlyContributor(false)

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"authorIsOnlyContributor",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"authorIsNotOnlyAuthor",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "ttest",
						Committer: "mhaypenny",
					},
				},
			},
		},
		{
			"authorIsNotOnlyCommitter",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "0cb194c52ee7c6c82110b59ec51b959ecfcb2fa2",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "9df0f1cee4b58363b534dbb5e9070fceee23fa10",
						Author:    "mhaypenny",
						Committer: "ttest",
					},
				},
			},
		},
	})
}

type AuthorTestCase struct {
	Name     string
	Expected bool
	Context  pull.Context
}

func runAuthorTests(t *testing.T, p Predicate, cases []AuthorTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ok, _, err := p.Evaluate(ctx, tc.Context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.Expected, ok, "predicate was not correct")
			}
		})
	}
}
