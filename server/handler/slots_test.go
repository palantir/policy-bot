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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/reviewer"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const slotPolicy = `
policy:
  approval: [{slot: service, required: true}]
slots:
  service: [Service review]
approval_rules:
  - name: Service review
    options:
      ignore_edited_comments: false
    requires:
      count: 1
      users: [reviewer]
`

func slotEvalContext(t *testing.T, input string) *EvalContext {
	t.Helper()
	var config policy.Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(input), &config))
	ec := makeEvalContext(true)
	ec.Config.LoadError = nil
	ec.Config.Config = &config
	return ec
}

func TestSlotEvaluationStatus(t *testing.T) {
	for _, test := range []struct {
		name, input, status string
		invalid             bool
	}{
		{"missing required", `policy: {approval: [{slot: missing, required: true}]}`, "error", true},
		{"empty optional", `policy: {approval: [{slot: empty}]}`, "error", false},
		{"empty approval", `policy: {approval: []}`, "success", false},
		{"filled pending", slotPolicy, "pending", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ec := slotEvalContext(t, test.input)
			evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			if test.invalid {
				require.Error(t, err)
				assert.Nil(t, evaluator)
			} else {
				require.NoError(t, err)
				require.NotNil(t, evaluator)
				_, err = ec.EvaluatePolicy(context.Background(), evaluator)
				require.NoError(t, err)
			}
			require.NotNil(t, ec.Status)
			assert.Equal(t, test.status, ec.Status.GetState())
		})
	}

	t.Run("filled evaluation error", func(t *testing.T) {
		ec := slotEvalContext(t, slotPolicy)
		failure := errors.New("comments unavailable")
		ec.PullContext.(*pulltest.Context).CommentsError = failure
		evaluator, err := ec.ParseConfig(context.Background(), common.TriggerComment)
		require.NoError(t, err)
		require.NotNil(t, evaluator)
		_, err = ec.EvaluatePolicy(context.Background(), evaluator)
		require.ErrorIs(t, err, failure)
		assert.Equal(t, "error", ec.Status.GetState())
	})
}

func TestSlotEventMethodsAndServerOverride(t *testing.T) {
	ec := slotEvalContext(t, slotPolicy)
	ec.Options.ApprovalDefaults = &approval.Defaults{Options: &approval.Options{
		Methods: &common.Methods{Comments: []string{"ship it"}},
	}}
	ec.Options.IgnoreEditedComments = new(true)
	prctx := ec.PullContext.(*pulltest.Context)
	prctx.CommentsValue = []*pull.Comment{{Author: "reviewer", Body: "ship it", LastEditedAt: time.Unix(1, 0)}}

	ctx := context.Background()
	evaluator, err := ec.ParseConfig(ctx, common.TriggerComment)
	require.NoError(t, err)
	require.NotNil(t, evaluator)
	commentHandler := &IssueComment{}
	event := github.IssueCommentEvent{
		Action:  new("edited"),
		Comment: &github.IssueComment{Body: new("removed")},
		Changes: &github.EditChange{Body: &github.EditBody{From: new("ship it")}},
	}
	assert.True(t, commentHandler.affectsApproval(event, evaluator.Methods()), "removing an approval must reevaluate")
	event.Action = new("created")
	assert.False(t, commentHandler.affectsApproval(event, evaluator.Methods()))
	result, err := ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)
	assert.Equal(t, common.StatusPending, result.Status, "server override rejects an edited approval")
	prctx.CommentsValue[0].LastEditedAt = time.Time{}
	result, err = ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)
	assert.Equal(t, common.StatusApproved, result.Status, "the inherited comment method approves an unedited comment")

	prctx.CommentsValue = nil
	prctx.ReviewsValue = []*pull.Review{{ID: "review-id", Author: "reviewer", State: pull.ReviewApproved}}
	evaluator, err = ec.ParseConfig(ctx, common.TriggerReview)
	require.NoError(t, err)
	require.NotNil(t, evaluator)
	reviewHandler := &PullRequestReview{}
	for _, review := range []*github.PullRequestReview{
		{State: new("approved")},
		{State: new("dismissed")},
		{State: new("commented"), Body: new("ship it")},
	} {
		assert.True(t, reviewHandler.affectsApproval(github.PullRequestReviewEvent{Review: review}, evaluator.Methods()))
	}
	assert.False(t, reviewHandler.affectsApproval(github.PullRequestReviewEvent{Review: &github.PullRequestReview{
		State: new("commented"), Body: new("unrelated"),
	}}, evaluator.Methods()))
	result, err = ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)
	assert.Equal(t, common.StatusApproved, result.Status)

	ec.Config.Config.ApprovalDefaults = &approval.Defaults{Options: &approval.Options{
		Methods: &common.Methods{Comments: []string{"policy approval"}, GithubReview: new(false)},
	}}
	evaluator, err = ec.ParseConfig(ctx, common.TriggerReview)
	require.NoError(t, err)
	assert.Nil(t, evaluator, "policy defaults disable the inherited review method")
	for _, method := range []string{"policy approval", "rule approval"} {
		if method == "rule approval" {
			ec.Config.Config.ApprovalRules[0].Options.Methods = &common.Methods{Comments: []string{method}}
		}
		evaluator, err = ec.ParseConfig(ctx, common.TriggerComment)
		require.NoError(t, err)
		require.NotNil(t, evaluator)
		prctx.CommentsValue = []*pull.Comment{{Author: "reviewer", Body: "ship it"}}
		result = evaluator.Evaluate(ctx, prctx)
		assert.Equal(t, common.StatusPending, result.Status, "overridden server methods do not approve")
		prctx.CommentsValue[0].Body = method
		result = evaluator.Evaluate(ctx, prctx)
		assert.Equal(t, common.StatusApproved, result.Status)
	}
}

func TestSlotReviewerRequestsRespectApprovedBranches(t *testing.T) {
	for _, extension := range []bool{false, true} {
		t.Run(fmt.Sprintf("extension=%t", extension), func(t *testing.T) {
			ec := slotEvalContext(t, `
policy:
  approval:
    - Always
    - or: [Automatic, {slot: service}]
slots:
  service: [Service]
approval_defaults:
  options:
    request_review: {enabled: true, mode: all-users}
approval_rules:
  - name: Always
    requires: {count: 1, users: [parent-reviewer]}
  - name: Automatic
  - name: Service
    requires: {count: 1, users: [child-reviewer]}
`)
			rules := ec.Config.Config.ApprovalRules
			if extension {
				ec.Config.Parent = &policy.Config{
					Policy:           ec.Config.Config.Policy,
					ApprovalDefaults: ec.Config.Config.ApprovalDefaults,
					ApprovalRules:    rules[:2],
				}
				ec.Config.Config.Policy = policy.Policy{ApprovalExtends: &appconfig.RemoteRef{Remote: "example/shared"}}
				ec.Config.Config.ApprovalRules = rules[2:]
			}
			evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			require.NoError(t, err)
			result := evaluator.Evaluate(context.Background(), ec.PullContext)
			require.NoError(t, result.Error)
			require.Equal(t, common.StatusPending, result.Status)
			requests := reviewer.FindRequests(&result)
			require.Len(t, requests, 1)
			assert.Equal(t, "Always", requests[0].Name)

			rules[1].Requires.Count = 1
			result = evaluator.Evaluate(context.Background(), ec.PullContext)
			requests = reviewer.FindRequests(&result)
			require.Len(t, requests, 3)
			assert.Equal(t, "Service", requests[2].Name)
			assert.Equal(t, []string{"child-reviewer"}, requests[2].ReviewRequestRule.Users)
		})
	}
}

func TestSlotDismissalPreservesApproversAndCandidates(t *testing.T) {
	for _, extension := range []bool{false, true} {
		t.Run(fmt.Sprintf("extension=%t", extension), func(t *testing.T) {
			ec := slotEvalContext(t, `
policy:
  approval: [{slot: service}]
slots:
  service: [Stale, Stale, Current]
approval_defaults:
  options:
    invalidate_on_push: true
approval_rules:
  - name: Stale
    requires: {count: 1, users: [reviewer]}
  - name: Current
    options: {invalidate_on_push: false}
    requires: {count: 1, users: [reviewer]}
`)
			if extension {
				parent := slotEvalContext(t, `
policy:
  approval: [{slot: service}, Current]
approval_rules:
  - name: Current
    requires: {count: 1, users: [reviewer]}
`)
				ec.Config.Parent = parent.Config.Config
				ec.Config.Config.Policy = policy.Policy{ApprovalExtends: &appconfig.RemoteRef{Remote: "example/shared"}}
				ec.Config.Config.Slots["service"] = []any{"Stale", "Stale"}
			}
			prctx := ec.PullContext.(*pulltest.Context)
			prctx.CommitsValue = []*pull.Commit{{SHA: prctx.HeadSHA()}}
			prctx.PushedAtValue = map[string]time.Time{prctx.HeadSHA(): time.Unix(2, 0)}
			prctx.ReviewsValue = []*pull.Review{{ID: "stale-review", Author: "reviewer", State: pull.ReviewApproved, CreatedAt: time.Unix(1, 0)}}
			evaluator, err := ec.ParseConfig(context.Background(), common.TriggerAll)
			require.NoError(t, err)
			result := evaluator.Evaluate(context.Background(), prctx)
			require.NoError(t, result.Error)
			assert.Equal(t, map[string]bool{"reviewer": true}, findAllApprovers(&result))
			require.Len(t, findAllDismissals(&result), 2)

			var dismissed []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Variables struct {
						Input struct {
							ReviewID string `json:"pullRequestReviewId"`
						} `json:"input"`
					} `json:"variables"`
				}
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
					return
				}
				dismissed = append(dismissed, request.Variables.Input.ReviewID)
				_, err := w.Write([]byte(`{"data":{"dismissPullRequestReview":{"clientMutationId":null}}}`))
				assert.NoError(t, err)
			}))
			defer server.Close()
			ec.V4Client = githubv4.NewEnterpriseClient(server.URL, server.Client())
			require.NoError(t, ec.dismissStaleReviewsForResult(context.Background(), result))
			assert.Empty(t, dismissed, "a current approver is protected by non-strict dismissal")
			ec.Options.StrictReviewDismissal = true
			require.NoError(t, ec.dismissStaleReviewsForResult(context.Background(), result))
			assert.Equal(t, []string{"stale-review"}, dismissed, "strict dismissal still deduplicates reviews")
		})
	}
}

func TestValidateSlots(t *testing.T) {
	for _, test := range []struct {
		name, input string
		status      int
	}{
		{"filled", slotPolicy, http.StatusOK},
		{"required missing", `policy: {approval: [{slot: service, required: true}]}`, http.StatusUnprocessableEntity},
		{"malformed unused fill", `slots: {unused: [missing]}`, http.StatusUnprocessableEntity},
		{"unsupported extension", `policy: {approval_extends: null}`, http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(test.input))
			response := httptest.NewRecorder()
			Validate().ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code, response.Body.String())
		})
	}
}
