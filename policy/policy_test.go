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

package policy

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/disapproval"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type StaticEvaluator common.Result

func (eval *StaticEvaluator) Trigger() common.Trigger {
	return common.TriggerStatic
}

func (eval *StaticEvaluator) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return common.Result(*eval)
}

func TestParsePolicy(t *testing.T) {

	t.Run("happyPath", func(t *testing.T) {
		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{},
			},
		}

		_, err := ParsePolicy(c, nil)
		assert.NoError(t, err)
	})

	t.Run("overridesEditedCommentValue", func(t *testing.T) {
		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{},
			},
		}

		_, err := ParsePolicy(c, &GlobalOptions{
			IgnoreEditedComments: ptr(true),
		})
		assert.NoError(t, err)
	})

	t.Run("withDisapprovalPolicy", func(t *testing.T) {
		disapprovalPolicy := &disapproval.Policy{}
		c := &Config{
			Policy: Policy{
				Approval:    approval.Policy{},
				Disapproval: disapprovalPolicy,
			},
		}

		eval, err := ParsePolicy(c, nil)
		assert.NoError(t, err)
		assert.NotNil(t, eval)

		evaluatorImpl, ok := eval.(evaluator)
		assert.True(t, ok)
		assert.Equal(t, disapprovalPolicy, evaluatorImpl.disapproval)
	})

	t.Run("withMultipleApprovalRules", func(t *testing.T) {
		rule1 := &approval.Rule{Name: "rule1"}
		rule2 := &approval.Rule{Name: "rule2"}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{
					"rule1",
					"rule2",
				},
			},
			ApprovalRules: []*approval.Rule{rule1, rule2},
		}

		_, err := ParsePolicy(c, nil)
		assert.NoError(t, err)
	})

	t.Run("withComplexApprovalPolicy", func(t *testing.T) {
		rule1 := &approval.Rule{Name: "rule1"}
		rule2 := &approval.Rule{Name: "rule2"}
		rule3 := &approval.Rule{Name: "rule3"}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{
					map[interface{}]interface{}{
						"or": []interface{}{
							"rule1",
							map[interface{}]interface{}{
								"and": []interface{}{
									"rule2",
									"rule3",
								},
							},
						},
					},
				},
			},
			ApprovalRules: []*approval.Rule{rule1, rule2, rule3},
		}

		_, err := ParsePolicy(c, nil)
		assert.NoError(t, err)
		assert.Len(t, c.ApprovalRules, 3)
	})

	t.Run("withGlobalOptionsAppliedToRules", func(t *testing.T) {
		rule := &approval.Rule{
			Name: "test-rule",
			Options: approval.Options{
				IgnoreEditedComments: ptr(false),
			},
		}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{
					"test-rule",
				},
			},
			ApprovalRules: []*approval.Rule{rule},
		}

		_, err := ParsePolicy(c, &GlobalOptions{
			IgnoreEditedComments: ptr(true),
		})
		assert.NoError(t, err)
		assert.True(t, rule.Options.IsIgnoreEditedComments())
	})

	t.Run("errorWhenRuleNotFound", func(t *testing.T) {
		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{
					"non-existent-rule",
				},
			},
		}

		_, err := ParsePolicy(c, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse approval policy")
	})

	t.Run("emptyApprovalRules", func(t *testing.T) {
		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{},
			},
		}

		eval, err := ParsePolicy(c, nil)
		assert.NoError(t, err)
		assert.NotNil(t, eval)
	})

	t.Run("maximumRecursiveDepth", func(t *testing.T) {
		rule := &approval.Rule{Name: "rule"}

		var nestedPolicy interface{} = "rule"
		for i := 0; i < 12; i++ {
			nestedPolicy = map[interface{}]interface{}{
				"and": []interface{}{nestedPolicy},
			}
		}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{nestedPolicy},
			},
			ApprovalRules: []*approval.Rule{rule},
		}

		_, err := ParsePolicy(c, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reached maximum recursive depth")
	})

	t.Run("invalidConjunction", func(t *testing.T) {
		rule := &approval.Rule{Name: "rule"}

		invalidConjunction := map[interface{}]interface{}{
			"and": []interface{}{"rule"},
			"or":  []interface{}{"rule"},
		}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{invalidConjunction},
			},
			ApprovalRules: []*approval.Rule{rule},
		}

		_, err := ParsePolicy(c, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple keys found when one was expected")
	})

	t.Run("emptySubconditions", func(t *testing.T) {
		emptySubconditions := map[interface{}]interface{}{
			"and": []interface{}{},
		}

		c := &Config{
			Policy: Policy{
				Approval: approval.Policy{emptySubconditions},
			},
		}

		_, err := ParsePolicy(c, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty list of subconditions is not allowed")
	})
}

func TestEvaluator(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{}

	t.Run("disapprovalWins", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status: common.StatusApproved,
			},
			disapproval: &StaticEvaluator{
				Status:            common.StatusDisapproved,
				StatusDescription: "disapproved by test",
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, common.StatusDisapproved, r.Status)
		assert.Equal(t, "disapproved by test", r.StatusDescription)
	})

	t.Run("approvalWinsByDefault", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status:            common.StatusPending,
				StatusDescription: "2 approvals needed",
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusSkipped,
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, common.StatusPending, r.Status)
		assert.Equal(t, "2 approvals needed", r.StatusDescription)
	})

	t.Run("propagateError", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Error: errors.New("approval failed"),
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusDisapproved,
			},
		}

		r := eval.Evaluate(ctx, prctx)

		assert.EqualError(t, r.Error, "approval failed")
		assert.Equal(t, common.StatusSkipped, r.Status)
	})

	t.Run("setsProperties", func(t *testing.T) {
		eval := evaluator{
			approval: &StaticEvaluator{
				Status: common.StatusPending,
			},
			disapproval: &StaticEvaluator{
				Status: common.StatusDisapproved,
			},
		}

		r := eval.Evaluate(ctx, prctx)
		require.NoError(t, r.Error)

		assert.Equal(t, "policy", r.Name)
		if assert.Len(t, r.Children, 2) {
			assert.Equal(t, castToResult(eval.approval), r.Children[0])
			assert.Equal(t, castToResult(eval.disapproval), r.Children[1])
		}
	})
}

func TestConfigMarshalYaml(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name:   "empty",
			config: Config{},
		},
		{
			name: "withDisapproval",
			config: Config{
				Policy: Policy{
					Disapproval: &disapproval.Policy{},
				},
			},
			expected: `policy:
  disapproval: {}
`,
		},
		{
			name: "withApprovalRules",
			config: Config{
				ApprovalRules: []*approval.Rule{
					{
						Name: "rule1",
					},
				},
			},
			expected: `approval_rules:
- name: rule1
`,
		},
		{
			name: "withChangedFiles",
			config: Config{
				ApprovalRules: []*approval.Rule{
					{
						Name: "rule1",
						Predicates: predicate.Predicates{
							ChangedFiles: &predicate.ChangedFiles{
								Paths: []common.Regexp{
									common.NewCompiledRegexp(regexp.MustCompile(`^\.github/workflows/.*\.yml$`)),
								},
							},
						},
					},
				},
			},
			expected: `approval_rules:
- name: rule1
  if:
    changed_files:
      paths:
      - ^\.github/workflows/.*\.yml$
`,
		},
		{
			name: "author",
			config: Config{
				ApprovalRules: []*approval.Rule{
					{
						Name: "rule1",
						Predicates: predicate.Predicates{
							HasAuthorIn: &predicate.HasAuthorIn{
								Actors: common.Actors{
									Users: []string{"author1", "author2"},
								},
							},
							AuthorIsOnlyContributor: new(predicate.AuthorIsOnlyContributor),
						},
					},
				},
			},
			expected: `approval_rules:
- name: rule1
  if:
    has_author_in:
      users:
      - author1
      - author2
    author_is_only_contributor: false
`,
		},
		{
			name: "modifiedLines",
			config: Config{
				ApprovalRules: []*approval.Rule{
					{
						Name: "rule1",
						Predicates: predicate.Predicates{
							ModifiedLines: &predicate.ModifiedLines{
								Additions: predicate.ComparisonExpr{
									Op:    predicate.OpGreaterThan,
									Value: 10,
								},
							},
						},
					},
				},
			},
			expected: `approval_rules:
- name: rule1
  if:
    modified_lines:
      additions: '> 10'
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected := test.expected
			if expected == "" {
				expected = "{}\n"
			}

			b, err := yaml.Marshal(test.config)
			require.NoError(t, err)
			require.Equal(t, expected, string(b))
		})
	}
}

func castToResult(e common.Evaluator) *common.Result {
	return (*common.Result)(e.(*StaticEvaluator))
}

func ptr[T any](val T) *T {
	return &val
}
