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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/simulated"
	"github.com/palantir/policy-bot/pull"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulateSlotsWithoutMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, "/graphql", r.URL.Path, "simulation must not post statuses or request reviewers") {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var request struct{ Query string }
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			return
		}
		if !assert.True(t, strings.HasPrefix(request.Query, "query"), "simulation must not dismiss reviews") {
			http.Error(w, "unexpected mutation", http.StatusBadRequest)
			return
		}
		_, err := w.Write([]byte(`{"data":{"repository":{"pullRequest":{"commits":{"nodes":[{"commit":{"oid":"head"}}]},"comments":{"nodes":[]},"reviews":{"nodes":[]}}}}}`))
		assert.NoError(t, err)
	}))
	defer server.Close()
	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&baseURL, nil))
	require.NoError(t, err)
	input := strings.Replace(slotPolicy, "ignore_edited_comments: false", "ignore_edited_comments: false\n      request_review: {enabled: true, mode: all-users}", 1)
	var parentInput string
	h := &Simulate{Base: Base{
		ClientCreator: testClients{client: client, v4client: githubv4.NewEnterpriseClient(server.URL+"/graphql", server.Client())},
		PullOpts:      &PullEvaluationOptions{IgnoreEditedComments: new(true)},
		ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
			loadConfig: func(context.Context, *github.Client, string, string, string) (appconfig.Config, error) {
				return appconfig.Config{Content: []byte(input), Source: "example/service", Path: ".policy.yml"}, nil
			},
			loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
				return appconfig.Config{Content: []byte(parentInput), Source: "example/shared", Path: ".policy.yml"}, nil
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
	options := simulated.Options{AddReviews: []simulated.Review{{
		ID: "simulated-review", Author: "reviewer", State: string(pull.ReviewApproved),
		CreatedAt: new(time.Unix(1, 0)), LastEditedAt: new(time.Unix(2, 0)),
	}}}
	ctx := context.Background()
	result, err := h.getSimulatedResult(ctx, githubapp.Installation{ID: 1}, loc, options)
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, common.StatusPending, result.Status)
	leaf := result.Children[0].Children[0].Children[0]
	require.NotNil(t, leaf.ReviewRequestRule)
	require.Len(t, leaf.Dismissals, 1)
	assert.Equal(t, "simulated-review", leaf.Dismissals[0].Candidate.ReviewID)

	parentInput = "policy: {approval: [{slot: service, required: true}]}"
	input = strings.Replace(input, "policy:\n  approval: [{slot: service, required: true}]", extensionPolicy, 1)
	extendedResult, err := h.getSimulatedResult(ctx, githubapp.Installation{ID: 1}, loc, options)
	require.NoError(t, err)
	require.NoError(t, extendedResult.Error)
	assert.Equal(t, result, extendedResult, "child fills preserve options, requests, and dismissals")

	simulatedContext, fetched, err := h.newSimulatedContext(ctx, 1, loc, options)
	require.NoError(t, err)
	ec := &EvalContext{Config: *fetched, Options: h.PullOpts, PullContext: simulatedContext, SkipPostStatus: true}
	parsed, err := ec.ParseConfig(ctx, common.TriggerAll)
	require.NoError(t, err)
	normal, err := ec.EvaluatePolicy(ctx, parsed)
	require.NoError(t, err)
	assert.Equal(t, extendedResult, &normal, "normal evaluation and simulation share the composed policy")
	assert.Equal(t, "pending", ec.Status.GetState())

	input = "policy: {approval: [{slot: service, required: true}]}"
	result, err = h.getSimulatedResult(ctx, githubapp.Installation{ID: 1}, loc, options)
	require.Error(t, err)
	assert.Nil(t, result)
}

type testClients struct {
	githubapp.ClientCreator
	client   *github.Client
	v4client *githubv4.Client
}

func (c testClients) NewInstallationClient(int64) (*github.Client, error) {
	return c.client, nil
}

func (c testClients) NewInstallationV4Client(int64) (*githubv4.Client, error) {
	return c.v4client, nil
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
