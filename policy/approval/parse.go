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
	"fmt"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/pkg/errors"
)

type Policy []any

// Parse records declared slots in slots. A nil registry disallows declarations.
func (p Policy) Parse(rules map[string]*Rule, slots map[string]*SlotRule) (common.Evaluator, error) {
	root, err := p.parseTree(rules, slots)
	if err != nil {
		return nil, err
	}
	return &evaluator{root: root}, nil
}

// ParseFill parses an implicitly ANDed list of rules without allowing slots.
func (p Policy) ParseFill(rules map[string]*Rule) (common.Evaluator, error) {
	return p.parseTree(rules, nil)
}

func (p Policy) parseTree(rules map[string]*Rule, slots map[string]*SlotRule) (common.Evaluator, error) {
	if len(p) == 0 {
		return nil, nil
	}

	// assume "and" for the list of rules
	root := map[any]any{
		"and": []any(p),
	}

	return parsePolicyR(root, rules, slots, 0)
}

func parsePolicyR(policy any, rules map[string]*Rule, slots map[string]*SlotRule, depth int) (common.Evaluator, error) {
	if depth > 10 {
		return nil, errors.New("reached maximum recursive depth while processing policy")
	}

	// Base case
	if ruleName, ok := policy.(string); ok {
		if rule, ok := rules[ruleName]; ok {
			req := &RuleRequirement{
				rule: rule,
			}
			return req, nil
		}
		var keys []string
		for k := range rules {
			keys = append(keys, k)
		}
		return nil, errors.Errorf("policy references undefined rule '%s', allowed values: %v", ruleName, keys)
	}

	// Recursive case, we have "or:", "and:", etc.
	if conjunction, ok := policy.(map[any]any); ok {
		var ops []string
		for k := range conjunction {
			key, ok := k.(string)
			if !ok {
				return nil, errors.Errorf("expected string policy key, but got %T", k)
			}
			ops = append(ops, key)
		}
		if _, exists := conjunction["slot"]; exists {
			return parseSlot(conjunction, slots)
		}
		if len(ops) != 1 {
			return nil, errors.Errorf("multiple keys found when one was expected: %v", ops)
		}

		op := ops[0]
		if op != "and" && op != "or" {
			return nil, errors.Errorf("invalid conjunction '%s', allowed values: [or, and]", op)
		}
		values, ok := conjunction[op].([]any)
		if !ok {
			return nil, errors.Errorf("expected list of subconditions, but got %T", conjunction[op])
		}
		if len(values) == 0 {
			return nil, errors.Errorf("empty list of subconditions is not allowed")
		}

		var subrequirements []common.Evaluator
		for _, subpolicy := range values {
			subreq, err := parsePolicyR(subpolicy, rules, slots, depth+1)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("failed to parse subpolicies for '%s'", op))
			}
			subrequirements = append(subrequirements, subreq)
		}

		if op == "or" {
			return &OrRequirement{requirements: subrequirements}, nil
		}
		return &AndRequirement{requirements: subrequirements}, nil
	}

	return nil, errors.Errorf("malformed policy, expected string or map, but encountered %T", policy)
}

func parseSlot(marker map[any]any, slots map[string]*SlotRule) (common.Evaluator, error) {
	if slots == nil {
		return nil, errors.New("slot declarations are not allowed in slot fills")
	}
	for key := range marker {
		if key != "slot" && key != "required" {
			return nil, errors.Errorf("unknown slot key %q", key)
		}
	}
	slotName, ok := marker["slot"].(string)
	if !ok || slotName == "" {
		return nil, errors.New("slot name must be a nonempty string")
	}
	var required bool
	if value, exists := marker["required"]; exists {
		required, ok = value.(bool)
		if !ok {
			return nil, errors.Errorf("slot %q required flag must be a boolean", slotName)
		}
	}
	if _, exists := slots[slotName]; exists {
		return nil, errors.Errorf("duplicate slot name %q", slotName)
	}
	slot := &SlotRule{Name: slotName, Required: required}
	slots[slotName] = slot
	return slot, nil
}
