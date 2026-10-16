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

package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestSlotValidation(t *testing.T) {
	tests := map[string]string{
		"required missing under or": `policy: {approval: [{or: [rule, {slot: extra, required: true}]}]}`,
		"required empty": `policy: {approval: [{slot: extra, required: true}]}
slots: {extra: []}`,
		"duplicate declarations across branches": `policy: {approval: [{or: [{slot: extra}, {and: [{slot: extra}]}]}]}`,
		"unknown marker key":                     `policy: {approval: [{slot: extra, optional: true}]}`,
		"mixed conjunction and slot":             `policy: {approval: [{slot: extra, and: [rule]}]}`,
		"missing name":                           `policy: {approval: [{required: true}]}`,
		"empty name":                             `policy: {approval: [{slot: ''}]}`,
		"null name":                              `policy: {approval: [{slot: null}]}`,
		"numeric name":                           `policy: {approval: [{slot: 123}]}`,
		"boolean name":                           `policy: {approval: [{slot: true}]}`,
		"null required":                          `policy: {approval: [{slot: extra, required: null}]}`,
		"string required":                        `policy: {approval: [{slot: extra, required: 'false'}]}`,
		"numeric required":                       `policy: {approval: [{slot: extra, required: 0}]}`,
		"non-string marker key":                  `policy: {approval: [{slot: extra, 1: false}]}`,
		"non-string conjunction key":             `policy: {approval: [{1: [rule]}]}`,
		"unknown conjunction":                    `policy: {approval: [{xor: [rule]}]}`,
		"duplicate marker key":                   `policy: {approval: [{slot: extra, slot: other}]}`,
		"null fill":                              `slots: {extra: null}`,
		"scalar fill":                            `slots: {extra: rule}`,
		"map fill":                               `slots: {extra: {and: [rule]}}`,
		"empty fill name":                        `slots: {'': [rule]}`,
		"numeric fill name":                      `slots: {1: [rule]}`,
		"boolean fill name":                      `slots: {true: [rule]}`,
		"null fill name":                         `slots: {null: [rule]}`,
		"coercing duplicate names":               `slots: {1: [rule], '1': []}`,
		"duplicate fill keys":                    `slots: {extra: [rule], extra: []}`,
		"duplicate null fill keys":               `slots: {extra: null, extra: []}`,
		"unknown rule in matched fill": `policy: {approval: [{slot: extra}]}
slots: {extra: [missing]}`,
		"unknown rule in unmatched fill": `policy: {approval: [rule]}
slots: {unused: [missing]}`,
		"unknown rule without approval tree": `slots: {unused: [missing]}`,
		"nested slot in matched fill": `policy: {approval: [{slot: extra}]}
slots: {extra: [{or: [rule, {slot: nested}]}]}`,
		"nested slot in unmatched fill":  `slots: {unused: [{and: [{slot: nested}]}]}`,
		"malformed unmatched fill":       `slots: {unused: [{or: null}]}`,
		"non-string fill expression key": `slots: {unused: [{false: [rule]}]}`,
		"null rule entry": `policy: {approval: [rule]}
approval_rules: [null]`,
		"unsupported extension": `policy: {approval_extends: {remote: org/repo}}`,
		"null extension":        `policy: {approval_extends: null}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(input, "approval_rules:") {
				input += "\napproval_rules: [{name: rule}]"
			}
			var c Config
			err := yaml.UnmarshalStrict([]byte(input), &c)
			if err == nil {
				var eval common.Evaluator
				eval, err = ParsePolicy(&c, nil)
				assert.Nil(t, eval)
			}
			require.Error(t, err)
		})
	}
}

func TestRequiredSlotNames(t *testing.T) {
	var c Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(`policy:
  approval:
    - slot: z-last
      required: true
    - slot: a-first
      required: true
`), &c))
	_, err := ParsePolicy(&c, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `["a-first" "z-last"]`)
}

func TestSlotFillDepth(t *testing.T) {
	for _, depth := range []int{10, 11} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			var expr any = "rule"
			for i := 1; i < depth; i++ {
				expr = map[any]any{"or": []any{expr}}
			}
			c := Config{
				Policy:        Policy{Approval: approval.Policy{map[any]any{"slot": "extra"}}},
				Slots:         Slots{"extra": {expr}},
				ApprovalRules: []*approval.Rule{{Name: "rule"}},
			}
			_, err := ParsePolicy(&c, nil)
			if depth == 10 {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "maximum recursive depth")
			}
		})
	}
}

func TestSlotFillGrouping(t *testing.T) {
	eval := parseSlotPolicy(t, `
policy:
  approval: [Base, {or: [Emergency, {slot: extra, required: true}]}]
slots:
  extra: [A, {or: [B, C]}]
approval_rules:
  - name: Base
    requires: {count: 1, users: [base]}
  - name: Emergency
    requires: {count: 1, users: [emergency]}
  - name: A
    requires: {count: 1, users: [a]}
  - name: B
    requires: {count: 1, users: [b]}
  - name: C
    requires: {count: 1, users: [c]}
`)
	for _, tt := range []struct {
		users  []string
		status common.EvaluationStatus
	}{
		{[]string{"a", "c"}, common.StatusPending},
		{[]string{"base", "a"}, common.StatusPending},
		{[]string{"base", "b"}, common.StatusPending},
		{[]string{"base", "a", "c"}, common.StatusApproved},
		{[]string{"base", "emergency"}, common.StatusApproved},
	} {
		t.Run(fmt.Sprint(tt.users), func(t *testing.T) {
			prctx := &pulltest.Context{}
			for _, user := range tt.users {
				prctx.ReviewsValue = append(prctx.ReviewsValue, &pull.Review{Author: user, State: pull.ReviewApproved})
			}
			result := eval.Evaluate(context.Background(), prctx)
			require.NoError(t, result.Error)
			assert.Equal(t, tt.status, result.Status)
		})
	}
}

func TestEmptyAndUnusedSlots(t *testing.T) {
	rules := `
approval_rules:
  - name: review
    requires: {count: 1, users: [reviewer]}
  - name: conditional
    if: {has_labels: [security]}
`
	for _, tt := range []struct {
		name, input string
		status      common.EvaluationStatus
		trigger     common.Trigger
	}{
		{"absent approval", `slots: {unused: [review]}`, common.StatusApproved, common.TriggerCommit},
		{"omitted fill", `policy: {approval: [{slot: extra}]}`, common.StatusSkipped, common.TriggerCommit},
		{"empty fill", "policy: {approval: [{slot: extra}]}\nslots: {extra: []}", common.StatusSkipped, common.TriggerCommit},
		{"empty beside active", `policy: {approval: [{slot: extra}, review]}`, common.StatusPending, common.TriggerCommit | common.TriggerComment | common.TriggerReview},
		{"required populated but skipped", "policy: {approval: [{slot: extra, required: true}]}\nslots: {extra: [conditional]}", common.StatusSkipped, common.TriggerCommit | common.TriggerLabel},
		{"unmatched fill has no triggers", "policy: {approval: [{slot: extra}]}\nslots: {unused: [review]}", common.StatusSkipped, common.TriggerCommit},
		{"quoted numeric name", "policy: {approval: [{slot: '1'}]}\nslots: {'1': [review]}", common.StatusPending, common.TriggerCommit | common.TriggerComment | common.TriggerReview},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eval := parseSlotPolicy(t, tt.input+rules)
			assert.Equal(t, tt.trigger, eval.Trigger())
			result := eval.Evaluate(context.Background(), &pulltest.Context{})
			require.NoError(t, result.Error)
			assert.Equal(t, tt.status, result.Status)
		})
	}
}

func TestSlotEvaluationErrors(t *testing.T) {
	rules := `
slots: {extra: [label]}
approval_rules:
  - name: label
    if: {has_labels: [security]}
  - name: review
    requires: {count: 1, users: [reviewer]}
`
	for _, op := range []string{"and", "or"} {
		t.Run(op, func(t *testing.T) {
			eval := parseSlotPolicy(t, fmt.Sprintf(`policy: {approval: [{%s: [review, {slot: extra}]}]}`, op)+rules)
			for _, approved := range []bool{false, true} {
				prctx := &pulltest.Context{LabelsError: errors.New("labels unavailable")}
				if approved {
					prctx.ReviewsValue = []*pull.Review{{Author: "reviewer", State: pull.ReviewApproved}}
				}
				result := eval.Evaluate(context.Background(), prctx)
				if op == "and" {
					assert.Error(t, result.Error)
				} else {
					assert.NoError(t, result.Error)
					expected := common.StatusPending
					if approved {
						expected = common.StatusApproved
					}
					assert.Equal(t, expected, result.Status)
				}
				// OR may suppress the aggregate error, but retains child evidence.
				assert.Error(t, result.Children[0].Children[0].Children[1].Error)
			}
		})
	}
}

func parseSlotPolicy(t *testing.T, input string) common.Evaluator {
	t.Helper()
	var c Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(input), &c))
	eval, err := ParsePolicy(&c, nil)
	require.NoError(t, err)
	return eval
}
