// Copyright 2019 Palantir Technologies, Inc.
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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func assertPredicateResult(t *testing.T, expected, actual *common.PredicateResult) {
	assert.Equal(t, expected.Satisfied, actual.Satisfied, "predicate was not correct")
	assert.Equal(t, expected.Values, actual.Values, "values were not correct")
	assert.Equal(t, expected.ConditionsMap, actual.ConditionsMap, "conditions were not correct")
	assert.Equal(t, expected.ConditionValues, actual.ConditionValues, "conditions were not correct")
}

func TestUnmarshalAllowedConclusions(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    allowedConclusions
		expectedErr bool
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "single",
			input:    `["success"]`,
			expected: allowedConclusions{"success": unit{}},
		},
		{
			name:     "multiple",
			input:    `["success", "failure"]`,
			expected: allowedConclusions{"success": unit{}, "failure": unit{}},
		},
		{
			name:     "repeat",
			input:    `["success", "success"]`,
			expected: allowedConclusions{"success": unit{}},
		},
		{
			name:        "invalid",
			input:       `notyaml`,
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actual allowedConclusions
			err := yaml.UnmarshalStrict([]byte(tc.input), &actual)

			if tc.expectedErr {
				assert.Error(t, err, "UnmarshalStrict should have failed")
				return
			}

			assert.NoError(t, err, "UnmarshalStrict failed")
			assert.Equal(t, tc.expected, actual, "values were not correct")
		})
	}
}

func TestJoinWithOr(t *testing.T) {
	testCases := []struct {
		name     string
		input    allowedConclusions
		expected string
	}{
		{
			name:     "empty",
			input:    nil,
			expected: "",
		},
		{
			name:     "one conclusion",
			input:    allowedConclusions{"success": unit{}},
			expected: "success",
		},
		{
			name:     "two conclusions",
			input:    allowedConclusions{"success": unit{}, "failure": unit{}},
			expected: "failure or success",
		},
		{
			name:     "three conclusions",
			input:    allowedConclusions{"success": unit{}, "failure": unit{}, "cancelled": unit{}},
			expected: "cancelled, failure, or success",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.input.joinWithOr()
			assert.Equal(t, tc.expected, actual, "values were not correct")
		})
	}
}
