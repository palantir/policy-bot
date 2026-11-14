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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestSlotValidation(t *testing.T) {
	tests := map[string]struct{ input, message string }{
		"required missing under or": {`policy: {approval: [{or: [rule, {slot: {name: extra, required: true}}]}]}`, "required slots are empty"},
		"required empty": {`policy: {approval: [{slot: {name: extra, required: true}}]}
slots: {extra: []}`, "required slots are empty"},
		"required null": {`policy: {approval: [{slot: {name: extra, required: true}}]}
slots: {extra: null}`, "required slots are empty"},
		"duplicate declarations across branches": {`policy: {approval: [{or: [{slot: {name: extra}}, {and: [{slot: {name: extra}}]}]}]}`, `duplicate slot name "extra"`},
		"unknown declaration key":                {`policy: {approval: [{slot: {name: extra, optional: true}}]}`, `unknown slot key "optional"`},
		"mixed conjunction and slot":             {`policy: {approval: [{slot: {name: extra}, and: [rule]}]}`, "multiple keys found"},
		"flat declaration":                       {`policy: {approval: [{slot: extra}]}`, "expected slot definition map"},
		"null declaration":                       {`policy: {approval: [{slot: null}]}`, "expected slot definition map"},
		"missing name":                           {`policy: {approval: [{slot: {required: true}}]}`, "slot name must be a nonempty string"},
		"empty name":                             {`policy: {approval: [{slot: {name: ''}}]}`, "slot name must be a nonempty string"},
		"null name":                              {`policy: {approval: [{slot: {name: null}}]}`, "slot name must be a nonempty string"},
		"numeric name":                           {`policy: {approval: [{slot: {name: 123}}]}`, "slot name must be a nonempty string"},
		"boolean name":                           {`policy: {approval: [{slot: {name: true}}]}`, "slot name must be a nonempty string"},
		"null required":                          {`policy: {approval: [{slot: {name: extra, required: null}}]}`, "required flag must be a boolean"},
		"string required":                        {`policy: {approval: [{slot: {name: extra, required: 'false'}}]}`, "required flag must be a boolean"},
		"numeric required":                       {`policy: {approval: [{slot: {name: extra, required: 0}}]}`, "required flag must be a boolean"},
		"non-string declaration key":             {`policy: {approval: [{slot: {name: extra, 1: false}}]}`, "expected string slot key"},
		"unknown conjunction":                    {`policy: {approval: [{xor: [rule]}]}`, "invalid conjunction 'xor'"},
		"empty content name":                     {`slots: {'': [rule]}`, "slot name must be nonempty"},
		"null content name":                      {`slots: {null: [rule]}`, "slot name must be nonempty"},
		"unknown rule in matched content": {`policy: {approval: [{slot: {name: extra}}]}
slots: {extra: [missing]}`, "undefined rule 'missing'"},
		"unknown rule in unmatched content": {`policy: {approval: [rule]}
slots: {unused: [missing]}`, "undefined rule 'missing'"},
		"undeclared slot": {`policy: {approval: [rule, {slot: {name: extra}}]}
slots: {extras: [rule]}`, `slot content "extras" does not match a declared slot`},
		"undeclared empty slot": {`policy: {approval: [{slot: {name: extra}}]}
slots: {extras: []}`, `slot content "extras" does not match a declared slot`},
		"content without approval tree":      {`slots: {unused: [rule]}`, `slot content "unused" does not match a declared slot`},
		"unknown rule without approval tree": {`slots: {unused: [missing]}`, "undefined rule 'missing'"},
		"nested slot in matched content": {`policy: {approval: [{slot: {name: extra}}]}
slots: {extra: [{or: [rule, {slot: {name: nested}}]}]}`, "slot declarations are not allowed in slot content"},
		"nested slot in unmatched content":  {`slots: {unused: [{and: [{slot: {name: nested}}]}]}`, "slot declarations are not allowed in slot content"},
		"malformed unmatched content":       {`slots: {unused: [{or: null}]}`, "expected list of subconditions"},
		"non-string content expression key": {`slots: {unused: [{false: [rule]}]}`, "expected string policy key"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var c Config
			require.NoError(t, yaml.UnmarshalStrict([]byte(test.input+"\napproval_rules: [{name: rule}]"), &c))
			eval, err := ParsePolicy(&c, nil)
			assert.ErrorContains(t, err, test.message)
			assert.Nil(t, eval)
		})
	}
}

func TestRequiredSlotNames(t *testing.T) {
	var c Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(`policy:
  approval:
    - slot:
        name: z-last
        required: true
    - slot:
        name: a-first
        required: true
`), &c))
	_, err := ParsePolicy(&c, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `["a-first" "z-last"]`)
}

func TestSlotContentGrouping(t *testing.T) {
	eval := parseSlotPolicy(t, `
policy:
  approval: [Base, {or: [Emergency, {slot: {name: extra, required: true}}]}]
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

func TestEmptySlots(t *testing.T) {
	rules := `
approval_rules:
  - name: review
    requires: {count: 1, users: [reviewer]}
  - name: conditional
    if: {has_labels: [security]}
`
	for _, tt := range []struct {
		name, input, slotName string
		status                common.EvaluationStatus
		trigger               common.Trigger
		empty                 bool
	}{
		{"omitted content", `policy: {approval: [{slot: {name: extra}}]}`, "extra", common.StatusSkipped, common.TriggerCommit, true},
		{"empty content", "policy: {approval: [{slot: {name: extra}}]}\nslots: {extra: []}", "extra", common.StatusSkipped, common.TriggerCommit, true},
		{"null content", "policy: {approval: [{slot: {name: extra}}]}\nslots: {extra: null}", "extra", common.StatusSkipped, common.TriggerCommit, true},
		{"empty beside active", `policy: {approval: [{slot: {name: extra}}, review]}`, "extra", common.StatusPending, common.TriggerCommit | common.TriggerComment | common.TriggerReview, true},
		{"required populated but skipped", "policy: {approval: [{slot: {name: extra, required: true}}]}\nslots: {extra: [conditional]}", "extra", common.StatusSkipped, common.TriggerCommit | common.TriggerLabel, false},
		{"numeric content name", "policy: {approval: [{slot: {name: '1'}}]}\nslots: {1: [review]}", "1", common.StatusPending, common.TriggerCommit | common.TriggerComment | common.TriggerReview, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eval := parseSlotPolicy(t, tt.input+rules)
			assert.Equal(t, tt.trigger, eval.Trigger())
			result := eval.Evaluate(context.Background(), &pulltest.Context{})
			require.NoError(t, result.Error)
			assert.Equal(t, tt.status, result.Status)
			slot := findSlotResult(&result, tt.slotName)
			require.NotNil(t, slot)
			assert.Equal(t, tt.empty, slot.Slot.Empty)
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
			eval := parseSlotPolicy(t, fmt.Sprintf(`policy: {approval: [{%s: [review, {slot: {name: extra}}]}]}`, op)+rules)
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
				slot := findSlotResult(&result, "extra")
				require.NotNil(t, slot)
				assert.Error(t, slot.Error)
			}
		})
	}
}

func findSlotResult(result *common.Result, name string) *common.Result {
	if result.Slot != nil && result.Slot.Name == name {
		return result
	}
	for _, child := range result.Children {
		if slot := findSlotResult(child, name); slot != nil {
			return slot
		}
	}
	return nil
}

func parseSlotPolicy(t *testing.T, input string) common.Evaluator {
	t.Helper()
	var c Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(input), &c))
	eval, err := ParsePolicy(&c, nil)
	require.NoError(t, err)
	return eval
}
