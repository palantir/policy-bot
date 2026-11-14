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
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueCommentAuditsPolicy(t *testing.T) {
	defaults := &approval.Defaults{Options: &approval.Options{
		Methods: &common.Methods{Comments: []string{"ship"}},
	}}
	const standalonePolicy = `
policy: {approval: [Review]}
approval_rules:
  - name: Review
    requires: {count: 1}
`
	for _, test := range []struct {
		name, input, parent, body string
		defaults                  *approval.Defaults
		failure                   bool
		errorMessage              string
	}{
		{name: "built-in default", input: standalonePolicy, body: ":+1:", failure: true},
		{name: "policy default", input: standalonePolicy + "\napproval_defaults: {options: {methods: {comment_patterns: ['^ship$']}}}", body: "ship", failure: true},
		{name: "server default", input: standalonePolicy, body: "ship", defaults: defaults, failure: true},
		{name: "unused rule without comment trigger", input: strings.Replace(standalonePolicy, "[Review]", "[]", 1), body: ":+1:", failure: true},
		{name: "disabled comments", input: standalonePolicy + "    options: {methods: {comments: []}}", body: "ship", defaults: defaults},
		{name: "explicit method", input: standalonePolicy + "    options: {methods: {comments: [local]}}", body: "local", defaults: defaults, failure: true},
		{name: "unrelated comment", input: standalonePolicy, body: "unrelated"},
		{name: "parent default", input: extensionPolicy, parent: slotPolicy + "\napproval_defaults: {options: {methods: {comments: [ship]}}}", body: "ship", failure: true},
		{name: "parent without comment trigger", input: extensionPolicy, parent: strings.Replace(slotPolicy, "count: 1", "count: 0", 1), body: "ship", defaults: defaults, failure: true},
		{name: "child override", input: extensionPolicy + childSlotContent, parent: slotPolicy, body: "child", failure: true},
		{name: "replaced parent method", input: extensionPolicy + childSlotContent, parent: slotPolicy, body: ":+1:"},
		{name: "missing required slot", input: extensionPolicy, parent: "policy: {approval: [{slot: {name: missing, required: true}}]}", body: ":+1:", errorMessage: "required slots are empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ec := extensionEvalContext(t, test.input, test.parent)
			ec.Options.ApprovalDefaults = test.defaults
			event := github.IssueCommentEvent{
				Action:  new("edited"),
				Sender:  &github.User{Login: new("editor")},
				Comment: &github.IssueComment{User: &github.User{Login: new("reviewer")}, Body: new("removed")},
				Changes: &github.EditChange{Body: &github.EditBody{From: &test.body}},
			}
			tampered, err := (&IssueComment{}).detectAndLogTampering(context.Background(), ec, event)
			if test.errorMessage != "" {
				require.ErrorContains(t, err, test.errorMessage)
				require.NotNil(t, ec.Status)
				assert.Equal(t, "error", ec.Status.GetState())
				return
			}
			require.NoError(t, err)
			assert.True(t, tampered)
			if test.failure {
				require.NotNil(t, ec.Status)
				assert.Equal(t, "failure", ec.Status.GetState())
			} else {
				assert.Nil(t, ec.Status)
			}
		})
	}
}
