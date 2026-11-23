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
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestTrigger(t *testing.T) {
	// the default trigger of an approval rule with review approvals
	policyTrigger := common.TriggerCommit | common.TriggerReview

	tests := map[string]struct {
		Payload     string
		Trigger     common.Trigger
		Handled     bool
		MatchPolicy bool
	}{
		"editedBase": {
			Payload:     `{"action": "edited", "changes": {"base": {"ref": {"from": "main"}, "sha": {"from": "abc123"}}}}`,
			Trigger:     common.TriggerAll,
			Handled:     true,
			MatchPolicy: true,
		},
		"editedTitleAndBody": {
			Payload: `{"action": "edited", "changes": {"title": {"from": "old"}, "body": {"from": "old"}}}`,
			Trigger: common.TriggerPullRequest,
			Handled: true,
		},
		"opened": {
			Payload:     `{"action": "opened"}`,
			Trigger:     common.TriggerCommit | common.TriggerPullRequest,
			Handled:     true,
			MatchPolicy: true,
		},
		"synchronize": {
			Payload:     `{"action": "synchronize"}`,
			Trigger:     common.TriggerCommit,
			Handled:     true,
			MatchPolicy: true,
		},
		"labeled": {
			Payload: `{"action": "labeled"}`,
			Trigger: common.TriggerLabel,
			Handled: true,
		},
		"closed": {
			Payload: `{"action": "closed"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var event github.PullRequestEvent
			require.NoError(t, json.Unmarshal([]byte(test.Payload), &event))

			trigger, ok := pullRequestTrigger(&event)
			assert.Equal(t, test.Handled, ok)
			if !test.Handled {
				return
			}
			assert.Equal(t, test.Trigger, trigger)
			assert.Equal(t, test.MatchPolicy, trigger.Matches(policyTrigger), "unexpected policy trigger match")
		})
	}
}
