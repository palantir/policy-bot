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
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
)

func TestTriggerForPullRequestAction(t *testing.T) {
	for _, tc := range []struct {
		name    string
		action  string
		trigger common.Trigger
		ok      bool
	}{
		{
			name:    "opened triggers commit and pull request evaluation",
			action:  "opened",
			trigger: common.TriggerCommit | common.TriggerPullRequest,
			ok:      true,
		},
		{
			name:    "reopened triggers commit and pull request evaluation",
			action:  "reopened",
			trigger: common.TriggerCommit | common.TriggerPullRequest,
			ok:      true,
		},
		{
			name:    "synchronize triggers commit evaluation",
			action:  "synchronize",
			trigger: common.TriggerCommit,
			ok:      true,
		},
		{
			name:    "ready for review does not trigger evaluation",
			action:  "ready_for_review",
			trigger: common.TriggerStatic,
			ok:      false,
		},
		{
			name:    "unknown action does not trigger evaluation",
			action:  "converted_to_draft",
			trigger: common.TriggerStatic,
			ok:      false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trigger, ok := triggerForPullRequestAction(tc.action)

			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.trigger, trigger)
		})
	}
}
