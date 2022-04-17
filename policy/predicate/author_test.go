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
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"ttest"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"authorInUsers",
			&pulltest.Context{
				AuthorValue: "mhaypenny",
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"authorInTeams",
			&pulltest.Context{
				AuthorValue: "mortonh",
				TeamMemberships: map[string][]string{
					"mortonh": {
						"coolorg/approvers",
						"testorg/team",
					},
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mortonh"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"authorInOrgs",
			&pulltest.Context{
				AuthorValue: "mortonh",
				OrgMemberships: map[string][]string{
					"mortonh": {
						"coolorg",
						"testorg",
					},
				},
			},
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mortonh"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"commitCommitterInUsers",
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"commitAuthorInTeam",
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"commitAuthorInOrg",
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
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
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"ttest"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"containsCommitAuthorNotInList",
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
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"ttest"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"committersInListButAuthorsAreNot",
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
			&common.PredicateResult{
				Satisfied: false,
				Values:    []string{"ttest1"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"commitAuthorInTeam",
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny", "ttest"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
				},
			},
		},
		{
			"commitAuthorInOrg",
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
			&common.PredicateResult{
				Satisfied: true,
				Values:    []string{"mhaypenny", "ttest"},
				ConditionsMap: map[string][]string{
					"Organizations": p.Organizations,
					"Teams":         p.Teams,
					"Users":         p.Users,
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
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are the only contributors"},
			},
		},
		{
			"authorIsOnlyContributorViaWeb",
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
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are the only contributors"},
			},
		},
		{
			"authorIsNotOnlyAuthor",
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
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are the only contributors"},
			},
		},
		{
			"authorIsNotOnlyCommitter",
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
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are the only contributors"},
			},
		},
	})
}

func TestAuthorIsNotOnlyContributor(t *testing.T) {
	p := AuthorIsOnlyContributor(false)

	runAuthorTests(t, p, []AuthorTestCase{
		{
			"authorIsOnlyContributor",
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
			&common.PredicateResult{
				Satisfied:       false,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are not the only contributors"},
			},
		},
		{
			"authorIsNotOnlyAuthor",
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
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are not the only contributors"},
			},
		},
		{
			"authorIsNotOnlyCommitter",
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
			&common.PredicateResult{
				Satisfied:       true,
				Values:          []string{"mhaypenny"},
				ConditionValues: []string{"they are not the only contributors"},
			},
		},
	})
}

type AuthorTestCase struct {
	Name                    string
	Context                 pull.Context
	ExpectedPredicateResult *common.PredicateResult
}

func runAuthorTests(t *testing.T, p Predicate, cases []AuthorTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			predicateResult, err := p.Evaluate(ctx, tc.Context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.ExpectedPredicateResult.Satisfied, predicateResult.Satisfied, "predicate was not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.Values, predicateResult.Values, "values were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionsMap, predicateResult.ConditionsMap, "conditions were not correct")
				assert.Equal(t, tc.ExpectedPredicateResult.ConditionValues, predicateResult.ConditionValues, "conditions were not correct")
			}
		})
	}
}
