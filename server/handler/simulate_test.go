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

package handler

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectPartialErrors(t *testing.T) {
	tests := map[string]struct {
		Result   *common.Result
		Expected []*PartialError
	}{
		"nil result": {
			Result:   nil,
			Expected: nil,
		},
		"no errors": {
			Result: &common.Result{
				Name:   "policy",
				Status: common.StatusApproved,
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusApproved},
					{Name: "rule-b", Status: common.StatusApproved},
				},
			},
			Expected: nil,
		},
		"top-level error only, no children": {
			Result: &common.Result{
				Name:   "policy",
				Status: common.StatusPending,
				Error:  fmt.Errorf("top-level failure"),
			},
			Expected: nil,
		},
		"or: errored child with pending sibling": {
			Result: &common.Result{
				Name:   "or",
				Status: common.StatusPending,
				Error:  nil,
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusPending},
					{Name: "rule-b", Error: fmt.Errorf("rule-b failed")},
				},
			},
			Expected: []*PartialError{
				{Rule: "rule-b", Error: "rule-b failed"},
			},
		},
		"or: errored child with approved sibling": {
			Result: &common.Result{
				Name:   "or",
				Status: common.StatusApproved,
				Error:  nil,
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusApproved},
					{Name: "rule-b", Error: fmt.Errorf("rule-b failed")},
				},
			},
			Expected: []*PartialError{
				{Rule: "rule-b", Error: "rule-b failed"},
			},
		},
		"and: errored child not suppressed": {
			Result: &common.Result{
				Name:   "and",
				Status: common.StatusPending,
				Error:  fmt.Errorf("child error propagated"),
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusApproved},
					{Name: "rule-b", Error: fmt.Errorf("rule-b failed")},
				},
			},
			Expected: nil,
		},
		"nested: policy -> approval -> or -> children": {
			Result: &common.Result{
				Name:   "policy",
				Status: common.StatusPending,
				Error:  nil,
				Children: []*common.Result{
					{
						Name:   "approval-rule",
						Status: common.StatusPending,
						Error:  nil,
						Children: []*common.Result{
							{
								Name:   "or",
								Status: common.StatusPending,
								Error:  nil,
								Children: []*common.Result{
									{Name: "rule-a", Status: common.StatusPending},
									{Name: "rule-b", Error: fmt.Errorf("deep error")},
								},
							},
						},
					},
				},
			},
			Expected: []*PartialError{
				{Rule: "rule-b", Error: "deep error"},
			},
		},
		"multiple suppressed errors": {
			Result: &common.Result{
				Name:   "or",
				Status: common.StatusPending,
				Error:  nil,
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusPending},
					{Name: "rule-b", Error: fmt.Errorf("rule-b failed")},
					{Name: "rule-c", Error: fmt.Errorf("rule-c failed")},
				},
			},
			Expected: []*PartialError{
				{Rule: "rule-b", Error: "rule-b failed"},
				{Rule: "rule-c", Error: "rule-c failed"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := collectPartialErrors(test.Result)
			assert.Equal(t, test.Expected, result)
		})
	}
}

func TestNewSimulationResponse(t *testing.T) {
	tests := map[string]struct {
		Result   *common.Result
		Expected *SimulationResponse
	}{
		"nil result": {
			Result:   nil,
			Expected: &SimulationResponse{},
		},
		"result with no errors": {
			Result: &common.Result{
				Name:              "my-policy",
				Description:       "a policy",
				StatusDescription: "all rules approved",
				Status:            common.StatusApproved,
			},
			Expected: &SimulationResponse{
				Name:              "my-policy",
				Description:       "a policy",
				StatusDescription: "all rules approved",
				Status:            "approved",
			},
		},
		"result with top-level error": {
			Result: &common.Result{
				Name:   "my-policy",
				Status: common.StatusPending,
				Error:  fmt.Errorf("something broke"),
			},
			Expected: &SimulationResponse{
				Name:   "my-policy",
				Status: "pending",
				Error:  "something broke",
			},
		},
		"result with partial errors": {
			Result: &common.Result{
				Name:              "or",
				Status:            common.StatusPending,
				StatusDescription: "None of the rules are satisfied",
				Error:             nil,
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusPending},
					{Name: "rule-b", Error: fmt.Errorf("rule-b failed")},
				},
			},
			Expected: &SimulationResponse{
				Name:              "or",
				Status:            "pending",
				StatusDescription: "None of the rules are satisfied",
				PartialErrors: []*PartialError{
					{Rule: "rule-b", Error: "rule-b failed"},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := newSimulationResponse(test.Result)
			assert.Equal(t, test.Expected, response)
		})
	}
}

func TestSimulationResponseJSON(t *testing.T) {
	t.Run("field names are correct", func(t *testing.T) {
		resp := &SimulationResponse{
			Name:              "my-policy",
			Description:       "a policy",
			StatusDescription: "all rules approved",
			Status:            "approved",
			Error:             "something broke",
			PartialErrors: []*PartialError{
				{Rule: "rule-b", Error: "rule-b failed"},
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		require.NoError(t, err)

		expectedKeys := []string{"name", "description:", "status_description", "status", "error", "partial_errors"}
		for _, key := range expectedKeys {
			assert.Contains(t, fields, key, "missing expected JSON field %q", key)
		}
	})

	t.Run("partial_errors omitted when empty", func(t *testing.T) {
		resp := &SimulationResponse{
			Name:   "my-policy",
			Status: "approved",
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		require.NoError(t, err)

		assert.NotContains(t, fields, "partial_errors")
	})
}
