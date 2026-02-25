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

func TestNewSimulationResponse(t *testing.T) {
	tests := map[string]struct {
		Result   *common.Result
		Expected *SimulationResponse
	}{
		"nil result": {
			Result:   nil,
			Expected: &SimulationResponse{},
		},
		"result with no errors and no children": {
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
		"result with children": {
			Result: &common.Result{
				Name:              "or",
				Status:            common.StatusPending,
				StatusDescription: "None of the rules are satisfied",
				Children: []*common.Result{
					{Name: "rule-a", Status: common.StatusPending},
					{Name: "rule-b", Status: common.StatusApproved, Error: fmt.Errorf("rule-b failed")},
				},
			},
			Expected: &SimulationResponse{
				Name:              "or",
				Status:            "pending",
				StatusDescription: "None of the rules are satisfied",
				Children: []*SimulationResponse{
					{Name: "rule-a", Status: "pending"},
					{Name: "rule-b", Status: "approved", Error: "rule-b failed"},
				},
			},
		},
		"nested tree": {
			Result: &common.Result{
				Name:   "policy",
				Status: common.StatusPending,
				Children: []*common.Result{
					{
						Name:   "approval-rule",
						Status: common.StatusPending,
						Children: []*common.Result{
							{
								Name:   "or",
								Status: common.StatusPending,
								Children: []*common.Result{
									{Name: "rule-a", Status: common.StatusPending},
									{Name: "rule-b", Status: common.StatusSkipped, Error: fmt.Errorf("deep error")},
								},
							},
						},
					},
				},
			},
			Expected: &SimulationResponse{
				Name:   "policy",
				Status: "pending",
				Children: []*SimulationResponse{
					{
						Name:   "approval-rule",
						Status: "pending",
						Children: []*SimulationResponse{
							{
								Name:   "or",
								Status: "pending",
								Children: []*SimulationResponse{
									{Name: "rule-a", Status: "pending"},
									{Name: "rule-b", Status: "skipped", Error: "deep error"},
								},
							},
						},
					},
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
			Children: []*SimulationResponse{
				{Name: "rule-a", Status: "approved"},
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		require.NoError(t, err)

		expectedKeys := []string{"name", "description", "status_description", "status", "error", "children"}
		for _, key := range expectedKeys {
			assert.Contains(t, fields, key, "missing expected JSON field %q", key)
		}
	})

	t.Run("children omitted when empty", func(t *testing.T) {
		resp := &SimulationResponse{
			Name:   "my-policy",
			Status: "approved",
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		require.NoError(t, err)

		assert.NotContains(t, fields, "children")
	})
}
