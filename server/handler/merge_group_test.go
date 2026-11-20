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

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClientCreator struct {
	githubapp.ClientCreator
	client *github.Client
}

func (cc fakeClientCreator) NewInstallationClient(installationID int64) (*github.Client, error) {
	return cc.client, nil
}

const mergeGroupPayload = `{
  "action": "checks_requested",
  "installation": {"id": 1},
  "repository": {"name": "testrepo", "owner": {"login": "testorg"}},
  "merge_group": {
    "head_sha": "abc123",
    "base_ref": "refs/heads/release/1.0"
  }
}`

func TestMergeGroup(t *testing.T) {
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
			rulesets := &fakeRulesetChecks{checks: []string{"policy-bot: ruleset:release"}}

			h := MergeGroup{Base: Base{
				ClientCreator: fakeClientCreator{client: gh.Client()},
				PullOpts: &PullEvaluationOptions{
					StatusCheckContext: "policy-bot",
					StatusChecks:       test.StatusChecks,
				},
				RulesetChecks: rulesets,
				ConfigFetcher: &ConfigFetcher{
					Loader: mockConfigLoader{
						loadConfig: func(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
							return appconfig.Config{
								Content: []byte("policy:\n  approval: []\n"),
								Source:  "testorg/testrepo@release/1.0",
								Path:    ".policy.yml",
							}, nil
						},
					},
					SeenPolicyCache: NewSeenPolicyCache(),
				},
			}}

			err := h.Handle(context.Background(), "merge_group", "delivery", []byte(mergeGroupPayload))
			require.NoError(t, err)

			assert.Equal(t, test.Expected, statusContextsOf(gh.Statuses()))
			assert.Zero(t, rulesets.calls, "merge groups must never look up ruleset checks")
		})
	}
}
