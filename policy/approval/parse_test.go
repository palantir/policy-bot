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
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/policy/common"
)

func TestParsePolicy(t *testing.T) {
	policyText := `
- rule1
- rule2
- or:
   - and:
      - rule3
      - rule4
   - rule5
- and:
  - rule6
`

	ruleText := `
- name: rule1
  options:
    allow_author: true
- name: rule2
  options:
    allow_author: true
    allow_contributor: true
- name: rule3
  options:
    allow_author: true
    allow_contributor: true
    # invalidate_on_push: true
- name: rule4
  options:
    allow_author: true
    # invalidate_on_push: true
- name: rule5
  options:
    allow_contributor: true
    # invalidate_on_push: true
- name: rule6
  options:
    allow_contributor: true
`

	var policy Policy
	err := yaml.UnmarshalStrict([]byte(policyText), &policy)
	require.NoError(t, err, "failed to unmarshal policy")

	var rules []*Rule
	err = yaml.UnmarshalStrict([]byte(ruleText), &rules)
	require.NoError(t, err, "failed to unmarshal rules")

	rulesByName := make(map[string]*Rule)
	for _, r := range rules {
		rulesByName[r.Name] = r
	}

	req, err := policy.Parse(rulesByName)
	require.NoError(t, err, "failed to parse policy")

	expected := &evaluator{
		root: &AndRequirement{
			requirements: []common.Evaluator{
				&RuleRequirement{
					rule: rules[0],
				},
				&RuleRequirement{
					rule: rules[1],
				},
				&OrRequirement{
					requirements: []common.Evaluator{
						&AndRequirement{
							requirements: []common.Evaluator{
								&RuleRequirement{
									rule: rules[2],
								},
								&RuleRequirement{
									rule: rules[3],
								},
							},
						},
						&RuleRequirement{
							rule: rules[4],
						},
					},
				},
				&AndRequirement{
					requirements: []common.Evaluator{
						&RuleRequirement{
							rule: rules[5],
						},
					},
				},
			},
		},
	}

	require.True(t, reflect.DeepEqual(expected, req))
}

func TestParsePolicyError_empty(t *testing.T) {
	// Empty list
	policy := `
- rule1
- or: []
`

	rules := `
  - name: rule1
`

	_, err := loadAndParsePolicy(t, policy, rules)
	require.Error(t, err)
}

func TestParsePolicyError_unknownRule(t *testing.T) {
	// Non-existing rule
	policy := `
- rule1
`

	rules := `
- name: rule2
`

	_, err := loadAndParsePolicy(t, policy, rules)
	require.Error(t, err)
}

func TestParsePolicyError_illegalType(t *testing.T) {
	// Illegal type
	policy := `
- or:
  - rule1
  - [a, b]
`

	rules := `
- name: rule1
`

	_, err := loadAndParsePolicy(t, policy, rules)
	require.Error(t, err)
}

func TestParsePolicyError_multikey(t *testing.T) {
	// Multiple keys
	policy := `
- or:
    - rule1
  and:
    - rule1
`

	rules := `
- name: rule1
`

	_, err := loadAndParsePolicy(t, policy, rules)
	require.Error(t, err)
}

func TestParsePolicyError_recursiveDepth(t *testing.T) {
	// Recursive depth 5 is allowed
	policy := `
- or:
   - or:
      - or:
         - or:
            - rule1
`

	rules := `
- name: rule1
`

	_, err := loadAndParsePolicy(t, policy, rules)
	require.NoError(t, err)

	// Recursive depth 6 is not allowed
	policy = `
- or:
   - or:
      - or:
         - or:
            - or:
               - rule1
`

	_, err = loadAndParsePolicy(t, policy, rules)
	require.Error(t, err)
}

func loadAndParsePolicy(t *testing.T, policyText string, ruleText string) (common.Evaluator, error) {
	var policy Policy
	err := yaml.UnmarshalStrict([]byte(policyText), &policy)
	require.NoError(t, err, "failed to unmarshal policy")

	var rules []*Rule
	err = yaml.UnmarshalStrict([]byte(ruleText), &rules)
	require.NoError(t, err, "failed to unmarshal rules")

	rulesByName := make(map[string]*Rule)
	for _, r := range rules {
		rulesByName[r.Name] = r
	}

	return policy.Parse(rulesByName)
}
