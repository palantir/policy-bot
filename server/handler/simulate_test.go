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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/simulated"
	"github.com/palantir/policy-bot/pull"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulateExtension(t *testing.T) {
	client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	})
	h := &Simulate{Base: Base{
		ClientCreator: testClients{client: client, v4client: githubv4.NewClient(client.Client())},
		PullOpts:      &PullEvaluationOptions{},
		ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
			loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
				return appconfig.Config{Content: []byte(extensionPolicy + "\nslots: {service: [Child]}\napproval_rules: [{name: Child}]"), Source: "example/service", Path: ".policy.yml"}, nil
			},
			loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
				return appconfig.Config{Content: []byte("policy: {approval: [{slot: {name: service, required: true}}]}"), Source: "example/shared", Path: ".policy.yml"}, nil
			},
		}},
	}}
	repo := &github.Repository{ID: new(int64(1)), Name: new("service"), Owner: &github.User{Login: new("example")}}
	loc := pull.Locator{Owner: "example", Repo: "service", Number: 1, Value: &github.PullRequest{
		Title: new("Change"), State: new("open"), Number: new(1), Draft: new(false),
		CreatedAt: &github.Timestamp{Time: time.Unix(1, 0)}, User: &github.User{Login: new("author")},
		Base:         &github.PullRequestBranch{Ref: new("main"), Repo: repo},
		Head:         &github.PullRequestBranch{Ref: new("feature"), SHA: new("head"), Repo: repo},
		ChangedFiles: new(1),
	}}
	result, err := h.getSimulatedResult(context.Background(), githubapp.Installation{ID: 1}, loc, simulated.Options{})
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, common.StatusApproved, result.Status)
	leaf := result.Children[0].Children[0]
	assert.Equal(t, "Child", leaf.Name)
	assert.Equal(t, &common.SlotResult{Name: "service"}, leaf.Slot)
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
		"empty slot": {
			Result: &common.Result{
				Name:              "service",
				Status:            common.StatusSkipped,
				StatusDescription: "Slot is empty",
				Slot:              &common.SlotResult{Name: "service", Empty: true},
			},
			Expected: &SimulationResponse{
				Name:              "service",
				Status:            "skipped",
				StatusDescription: "Slot is empty",
				Slot:              "service",
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
								Slot:   &common.SlotResult{Name: "service"},
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
								Slot:   "service",
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
			Slot:              "service",
			Children: []*SimulationResponse{
				{Name: "rule-a", Status: "approved"},
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		require.NoError(t, err)

		expectedKeys := []string{"name", "description", "status_description", "status", "error", "slot", "children"}
		for _, key := range expectedKeys {
			assert.Contains(t, fields, key, "missing expected JSON field %q", key)
		}
		assert.JSONEq(t, `"service"`, string(fields["slot"]))
	})

	t.Run("optional fields omitted when empty", func(t *testing.T) {
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
		assert.NotContains(t, fields, "slot")
	})
}
