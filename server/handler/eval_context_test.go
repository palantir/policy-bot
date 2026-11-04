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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigSuppressesStatusForUnseenPolicy(t *testing.T) {
	ec := makeEvalContext(false)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	assert.Nil(t, ec.Status)
}

func TestParseConfigPostsStatusForSeenPolicy(t *testing.T) {
	ec := makeEvalContext(true)

	evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.Error(t, err)
	assert.Nil(t, evaluator)
	require.NotNil(t, ec.Status)
	assert.Equal(t, "error", ec.Status.GetState())
	assert.Equal(t, "policy-bot: main", ec.Status.GetContext())
	assert.Equal(t, "Error loading policy from testorg/testrepo@main", ec.Status.GetDescription())
}

func makeEvalContext(seenPolicy bool) *EvalContext {
	return &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL: "https://policy-bot.example.com",
		PullContext: &pulltest.Context{
			OwnerValue:     "testorg",
			RepoValue:      "testrepo",
			NumberValue:    42,
			StateValue:     "open",
			HeadSHAValue:   "abc123",
			BranchBaseName: "main",
			BranchHeadName: "feature",
		},
		SkipPostStatus: true,
		Config: FetchedConfig{
			LoadError:  errors.New("request failed"),
			Source:     "testorg/testrepo@main",
			Path:       ".policy.yml",
			SeenPolicy: seenPolicy,
		},
	}
}

type staticEvaluator common.Result

func (e *staticEvaluator) Trigger() common.Trigger { return common.TriggerAll }

func (e *staticEvaluator) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	return common.Result(*e)
}

func TestEvaluatePolicyStatusState(t *testing.T) {
	tests := map[string]struct {
		Result   common.Result
		Expected string
	}{
		"approved":          {common.Result{Status: common.StatusApproved}, "success"},
		"disapproved":       {common.Result{Status: common.StatusDisapproved}, "failure"},
		"pendingAndWaiting": {common.Result{Status: common.StatusPending}, "pending"},
		"pendingAndFailed":  {common.Result{Status: common.StatusPending, Failed: true}, "failure"},
		"skipped":           {common.Result{Status: common.StatusSkipped}, "error"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ec := makeEvalContext(true)
			evaluator := staticEvaluator(test.Result)

			_, err := ec.EvaluatePolicy(context.Background(), &evaluator)
			require.NoError(t, err)
			require.NotNil(t, ec.Status)
			assert.Equal(t, test.Expected, ec.Status.GetState())
		})
	}
}

func TestPostStatusKeepsAnUnchangedStatus(t *testing.T) {
	posted := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	require.NoError(t, err)

	detailsURL := "https://policy-bot.example.com/details/testorg/testrepo/42"
	current := func(state, description string) map[string]*pull.CommitStatus {
		return map[string]*pull.CommitStatus{
			"policy-bot: main": {State: state, Description: description, TargetURL: detailsURL},
		}
	}

	tests := map[string]struct {
		Current            map[string]*pull.CommitStatus
		State, Description string
		Posts              bool
	}{
		"noStatusYet":        {nil, "pending", "Waiting", true},
		"samePending":        {current("pending", "Waiting"), "pending", "Waiting", false},
		"sameSuccess":        {current("success", "All rules are approved"), "success", "All rules are approved", false},
		"newDescription":     {current("pending", "Waiting: lint"), "pending", "Waiting", true},
		"newState":           {current("pending", "Waiting"), "success", "Waiting", true},
		"sameFailureReposts": {current("failure", "Failed: lint"), "failure", "Failed: lint", true},
		"sameErrorReposts":   {current("error", "Invalid policy"), "error", "Invalid policy", true},
		"overriddenPending":  {current("success", "Overridden by tommy"), "pending", "Waiting", false},
		"overriddenFailure":  {current("success", "Overridden by tommy"), "failure", "Failed: lint", false},
		"overriddenError":    {current("success", "Overridden by tommy"), "error", "Invalid policy", false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			posted = 0
			ec := makeEvalContext(true)
			ec.SkipPostStatus = false
			ec.Client = client
			ec.PullContext.(*pulltest.Context).CommitStatusesValue = test.Current

			ec.PostStatus(context.Background(), test.State, test.Description)
			assert.Equal(t, test.Posts, posted == 1)
		})
	}
}

func TestPostOverride(t *testing.T) {
	var posted []github.RepoStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var status github.RepoStatus
		require.NoError(t, json.NewDecoder(r.Body).Decode(&status))
		posted = append(posted, status)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	require.NoError(t, err)

	ec := makeEvalContext(true)
	ec.SkipPostStatus = false
	ec.Client = client
	ec.Options.OverrideUsers = []string{"Tommy"}

	assert.True(t, ec.Options.CanOverride("tommy"))
	assert.False(t, ec.Options.CanOverride("dan"))

	overriddenBy, err := ec.OverriddenBy()
	require.NoError(t, err)
	assert.Equal(t, "", overriddenBy)

	require.NoError(t, ec.PostOverride(context.Background(), "tommy"))
	require.Len(t, posted, 1)
	assert.Equal(t, "success", posted[0].GetState())
	assert.Equal(t, "policy-bot: main", posted[0].GetContext())
	assert.Equal(t, "Overridden by tommy", posted[0].GetDescription())

	ec.PullContext.(*pulltest.Context).CommitStatusesValue = map[string]*pull.CommitStatus{
		"policy-bot: main": {State: posted[0].GetState(), Description: posted[0].GetDescription()},
	}
	overriddenBy, err = ec.OverriddenBy()
	require.NoError(t, err)
	assert.Equal(t, "tommy", overriddenBy)
}
