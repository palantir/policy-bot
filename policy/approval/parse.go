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

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/policy/common"
)

type Policy []interface{}

func (p Policy) Parse(rules map[string]*Rule) (common.Evaluator, error) {
	eval := &evaluator{}

	if len(p) == 0 {
		return eval, nil
	}

	// assume "and" for the list of rules
	root := map[interface{}]interface{}{
		"and": []interface{}(p),
	}

	and, err := parsePolicyR(root, rules, 0)
	if err != nil {
		return nil, err
	}

	eval.root = and
	return eval, nil
}

func parsePolicyR(policy interface{}, rules map[string]*Rule, depth int) (common.Evaluator, error) {
	if depth > 5 {
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
	if conjunction, ok := policy.(map[interface{}]interface{}); ok {
		var ops []string
		for k := range conjunction {
			ops = append(ops, k.(string))
		}
		if len(ops) != 1 {
			return nil, errors.Errorf("multiple keys found when one was expected: %v", ops)
		}

		op := ops[0]
		values, ok := conjunction[op].([]interface{})
		if !ok {
			return nil, errors.Errorf("expected list of subconditions, but got %T", conjunction[op])
		}
		if len(values) == 0 {
			return nil, errors.Errorf("empty list of subconditions is not allowed")
		}

		var subrequirements []common.Evaluator
		for _, subpolicy := range values {
			subreq, err := parsePolicyR(subpolicy, rules, depth+1)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("failed to parse subpolicies for '%s'", op))
			}
			subrequirements = append(subrequirements, subreq)
		}

		switch op {
		case "or":
			return &OrRequirement{requirements: subrequirements}, nil
		case "and":
			return &AndRequirement{requirements: subrequirements}, nil
		default:
			return nil, errors.Errorf("invalid conjunction '%s', allowed values: [or, and]", op)
		}
	}

	return nil, errors.Errorf("malformed policy, expected string or map, but encountered %T", policy)
}
