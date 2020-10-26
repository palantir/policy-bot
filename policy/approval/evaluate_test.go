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

package approval

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestRules(t *testing.T) {
	ruleText := `
- name: rule1
  if:
    changed_files:
      paths: ["path1"]
    only_changed_files:
      paths: ["path2"]
    has_author_in:
      teams: ["team1"]
      users: ["user1", "user2"]
      organizations: ["org1"]
    has_contributor_in:
      teams: ["team2"]
      users: ["user3"]
      organizations: ["org2"]
  options:
    allow_author: true
    allow_contributor: true
    # invalidate_on_push: true
    methods:
      comments: ["+1"]
      github_review: true
  requires:
    users: ["user4"]
    teams: ["team3", "team4"]
    organizations: ["org3"]
    count: 5
`

	var rules []*Rule
	require.NoError(t, yaml.UnmarshalStrict([]byte(ruleText), &rules))

	expected := []*Rule{
		{
			Name: "rule1",
			Predicates: Predicates{
				ChangedFiles: &predicate.ChangedFiles{
					Paths: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("path1")),
					},
				},
				OnlyChangedFiles: &predicate.OnlyChangedFiles{
					Paths: []common.Regexp{
						common.NewCompiledRegexp(regexp.MustCompile("path2")),
					},
				},
				HasAuthorIn: &predicate.HasAuthorIn{
					Actors: common.Actors{
						Teams:         []string{"team1"},
						Users:         []string{"user1", "user2"},
						Organizations: []string{"org1"},
					},
				},
				HasContributorIn: &predicate.HasContributorIn{
					Actors: common.Actors{
						Teams:         []string{"team2"},
						Users:         []string{"user3"},
						Organizations: []string{"org2"},
					},
				},
			},
			Options: Options{
				AllowAuthor:      true,
				AllowContributor: true,
				// InvalidateOnPush: true,
				Methods: &common.Methods{
					Comments:     []string{"+1"},
					GithubReview: true,
				},
			},
			Requires: Requires{
				Count: 5,
				Actors: common.Actors{
					Users:         []string{"user4"},
					Teams:         []string{"team3", "team4"},
					Organizations: []string{"org3"},
				},
			},
		},
	}

	require.True(t, reflect.DeepEqual(expected, rules))
}

type mockRequirement struct {
	result *common.Result
}

func (m *mockRequirement) Trigger() common.Trigger {
	return common.TriggerStatic
}

func (m *mockRequirement) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return *m.result
}

func makeRulesResultingIn(es ...common.EvaluationStatus) []common.Evaluator {
	var requirements []common.Evaluator
	for _, e := range es {
		requirements = append(requirements, &mockRequirement{
			result: &common.Result{
				Status: e,
			},
		})
	}
	return requirements
}

func TestAndRequirement(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{}

	// One pending is pending
	and := &AndRequirement{
		requirements: makeRulesResultingIn(common.StatusApproved, common.StatusPending),
	}
	result := and.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusPending, result.Status)

	// All approved is approved
	and = &AndRequirement{
		requirements: makeRulesResultingIn(common.StatusApproved),
	}
	result = and.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)

	// Skipped counts
	and = &AndRequirement{
		requirements: makeRulesResultingIn(common.StatusApproved, common.StatusSkipped),
	}
	result = and.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)

	// Skipped itself is results in Skipped
	and = &AndRequirement{
		requirements: makeRulesResultingIn(common.StatusSkipped),
	}
	result = and.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusSkipped, result.Status)

	// Error blocks approval
	and = &AndRequirement{
		requirements: []common.Evaluator{
			&mockRequirement{
				result: &common.Result{
					Status: common.StatusApproved,
				},
			},
			&mockRequirement{
				result: &common.Result{
					Error: errors.New("error"),
				},
			},
		},
	}
	result = and.Evaluate(ctx, prctx)
	assert.Error(t, result.Error)
}

func TestOrRequirement(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{}

	// One approval is enough
	or := &OrRequirement{
		requirements: makeRulesResultingIn(common.StatusPending, common.StatusSkipped, common.StatusApproved),
	}
	result := or.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)

	// All approved is approved
	or = &OrRequirement{
		requirements: makeRulesResultingIn(common.StatusApproved),
	}
	result = or.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)

	// Skipped does not affect approval
	or = &OrRequirement{
		requirements: makeRulesResultingIn(common.StatusApproved, common.StatusSkipped),
	}
	result = or.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)

	// Skipped itself results in Skipped
	or = &OrRequirement{
		requirements: makeRulesResultingIn(common.StatusSkipped),
	}
	result = or.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusSkipped, result.Status)

	// Error does not block approval
	or = &OrRequirement{
		requirements: []common.Evaluator{
			&mockRequirement{
				result: &common.Result{
					Status: common.StatusApproved,
				},
			},
			&mockRequirement{
				result: &common.Result{
					Error: errors.New("error"),
				},
			},
		},
	}
	result = or.Evaluate(ctx, prctx)
	assert.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)
}
