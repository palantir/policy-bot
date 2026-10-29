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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const extensionPolicy = `policy: {approval_extends: {remote: example/shared, path: .policy.yml, ref: main}}`

const childSlotFill = `
slots: {service: [Child]}
approval_defaults:
  options:
    methods: {comments: [child], github_review: false}
approval_rules:
  - name: Child
    requires: {count: 1, users: [reviewer]}
`

func extensionEvalContext(t *testing.T, child, parent string) *EvalContext {
	t.Helper()
	ec := slotEvalContext(t, child)
	ec.Config.Parent = new(policy.Config)
	require.NoError(t, yaml.UnmarshalStrict([]byte(parent), ec.Config.Parent))
	return ec
}

func TestExtensionEventMethods(t *testing.T) {
	parent := slotPolicy + `
approval_defaults:
  options:
    methods: {comments: [parent], github_review: false}
`
	for _, test := range []struct {
		name, child string
		matches     []string
		excludes    []string
	}{
		{"parent default", extensionPolicy, []string{"parent"}, []string{"child"}},
		{"child override", extensionPolicy + childSlotFill, []string{"child"}, []string{"parent", ":+1:"}},
		{"child disapproval", `policy:
  approval_extends: {remote: example/shared}
  disapproval: {options: {methods: {disapprove: {comments: [block]}, revoke: {comments: [unblock]}}}}`, []string{"parent", "block", "unblock"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			ec := extensionEvalContext(t, test.child, parent)
			parsed, err := ec.ParseConfig(context.Background(), common.TriggerComment)
			require.NoError(t, err)
			require.NotNil(t, parsed)
			for _, body := range append(test.matches, test.excludes...) {
				expected := false
				for _, match := range test.matches {
					expected = expected || body == match
				}
				assert.Equal(t, expected, (&IssueComment{}).affectsApproval(github.IssueCommentEvent{
					Action: new("created"), Comment: &github.IssueComment{Body: &body},
				}, parsed.Methods()), body)
				assert.Equal(t, expected, (&PullRequestReview{}).affectsApproval(github.PullRequestReviewEvent{
					Review: &github.PullRequestReview{State: new("commented"), Body: &body},
				}, parsed.Methods()), body)
			}
		})
	}
}

func TestValidateExtensionOffline(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(extensionPolicy))
	response := httptest.NewRecorder()
	Validate().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Contains(t, response.Body.String(), "parent")
	assert.Contains(t, response.Body.String(), "simulation")

	request = httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader("remote: example/shared"))
	response = httptest.NewRecorder()
	Validate().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
}
