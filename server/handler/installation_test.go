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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const installationPayload = `{
  "action": "created",
  "installation": {"id": 1},
  "repositories": [{"full_name": "testorg/testrepo"}]
}`

func TestInstallation(t *testing.T) {
	tests := map[string]struct {
		StatusChecks []StatusCheckType
		Expected     []string
	}{
		"branch": {
			StatusChecks: []StatusCheckType{StatusCheckBranch},
			Expected:     []string{"policy-bot: release/1.0"},
		},
		"branchAndRuleset": {
			StatusChecks: []StatusCheckType{StatusCheckBranch, StatusCheckRuleset},
			Expected:     []string{"policy-bot: release/1.0"},
		},
		"rulesetOnly": {
			StatusChecks: []StatusCheckType{StatusCheckRuleset},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gh := newFakeGitHub(t)
			gh.Respond("/repos/testorg/testrepo", 0, fakeResponse{body: map[string]any{"default_branch": "release/1.0"}})
			gh.Respond("/repos/testorg/testrepo/branches/release%2F1.0", 0, fakeResponse{body: map[string]any{"commit": map[string]any{"sha": "abc123"}}})
			rulesets := &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}}

			h := Installation{Base: Base{
				ClientCreator: fakeClientCreator{client: gh.Client()},
				PullOpts: &PullEvaluationOptions{
					StatusCheckContext: "policy-bot",
					StatusChecks:       test.StatusChecks,
				},
				RulesetChecks: rulesets,
				AppName:       "policy-bot",
			}}

			err := h.Handle(context.Background(), "installation", "delivery", []byte(installationPayload))
			require.NoError(t, err)

			assert.Equal(t, test.Expected, statusContextsOf(gh.Statuses()))
			assert.Zero(t, rulesets.calls, "installation must never look up ruleset checks")
		})
	}
}
