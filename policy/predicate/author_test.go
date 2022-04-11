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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
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
			&common.PredicateInfo{
				Type: "HasAuthorIn",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "ttest",
					Contributors:  []string{},
				},
			},
		},
		{
			"authorInUsers",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
			},
			&common.PredicateInfo{
				Type: "HasAuthorIn",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "mhaypenny",
					Contributors:  []string{},
				},
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
			&common.PredicateInfo{
				Type: "HasAuthorIn",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "mortonh",
					Contributors:  []string{},
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
			&common.PredicateInfo{
				Type: "HasAuthorIn",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "mortonh",
					Contributors:  []string{},
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
			&common.PredicateInfo{
				Type: "HasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"mhaypenny"},
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
			&common.PredicateInfo{
				Type: "HasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"mhaypenny"},
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
			&common.PredicateInfo{
				Type: "HasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"mhaypenny"},
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
			&common.PredicateInfo{
				Type: "HasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"mhaypenny"},
				},
			},
		},
	})
}

func TestOnlyHasContributorsIn(t *testing.T) {
	p := &OnlyHasContributorsIn{
		common.Actors{
			Teams:         []string{"testorg/team"},
			Users:         []string{"mhaypenny"},
			Organizations: []string{"testorg"},
		},
	}

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"authorNotInList",
			false,
			&pulltest.Context{
				AuthorValue: "ttest",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
				},
			},
			&common.PredicateInfo{
				Type: "OnlyHasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"ttest"},
				},
			},
		},
		{
			"containsCommitAuthorNotInList",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
					},
				},
			},
			&common.PredicateInfo{
				Type: "OnlyHasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"ttest"},
				},
			},
		},
		{
			"committersInListButAuthorsAreNot",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest1",
						Committer: "mhaypenny",
					},
					{
						SHA:       "abcdef123456789",
						Author:    "ttest2",
						Committer: "mhaypenny",
					},
				},
			},
			&common.PredicateInfo{
				Type: "OnlyHasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"ttest1", "ttest2"},
				},
			},
		},
		{
			"commitAuthorInTeam",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				TeamMemberships: map[string][]string{
					"ttest": {
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
			&common.PredicateInfo{
				Type: "OnlyHasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"ttest", "mhaypenny"},
				},
			},
		},
		{
			"commitAuthorInOrg",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				OrgMemberships: map[string][]string{
					"ttest": {
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
			&common.PredicateInfo{
				Type: "OnlyHasContributorsIn",
				Name: "Contributors",
				ContributorInfo: &common.ContributorInfo{
					Organizations: p.Organizations,
					Teams:         p.Teams,
					Users:         p.Users,
					Author:        "",
					Contributors:  []string{"ttest", "mhaypenny"},
				},
			},
		},
	})
}

func TestAuthorIsOnlyContributor(t *testing.T) {
	p := AuthorIsOnlyContributor(true)

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"AuthorIsOnlyContributor",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
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
			&common.PredicateInfo{
				Type: "AuthorIsOnlyContributor",
				Name: "Author",
				ContributorInfo: &common.ContributorInfo{
					Author: "mhaypenny",
				},
			},
		},
	})
}

type AuthorTestCase struct {
	Name                  string
	Expected              bool
	Context               pull.Context
	ExpectedPredicateInfo *common.PredicateInfo
}

func isContributorInfoEqual() {

}

func runAuthorTests(t *testing.T, p Predicate, cases []AuthorTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ok, _, predicateInfo, err := p.Evaluate(ctx, tc.Context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.Expected, ok, "predicate was not correct")
				if tc.ExpectedPredicateInfo.ContributorInfo.Contributors != nil {
					assert.Subset(t, tc.ExpectedPredicateInfo.ContributorInfo.Contributors, predicateInfo.ContributorInfo.Contributors, "ContributorInfo was not correct")
					tc.ExpectedPredicateInfo.ContributorInfo.Contributors = nil
					predicateInfo.ContributorInfo.Contributors = nil
				}
				assert.Equal(t, *tc.ExpectedPredicateInfo.ContributorInfo, *predicateInfo.ContributorInfo, "ContributorInfo was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Name, predicateInfo.Name, "PredicateInfo's Name was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Type, predicateInfo.Type, "PredicateInfo's Type was not correct")
			}
		})
	}
}
