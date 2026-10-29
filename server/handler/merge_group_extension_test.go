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
	"net/http"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeGroupWithUnavailableParent(t *testing.T) {
	for _, test := range []struct {
		name, parent string
		loadError    error
	}{
		{name: "parent denied", loadError: githubErr(http.StatusForbidden)},
		{name: "parent malformed", parent: "policy: ["},
	} {
		t.Run(test.name, func(t *testing.T) {
			statuses := make(chan github.RepoStatus, 2)
			client := extensionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/repos/original/service/statuses/merge-head", r.URL.Path)
				var status github.RepoStatus
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&status))
				statuses <- status
				assert.NoError(t, json.NewEncoder(w).Encode(status))
			})
			h := MergeGroup{Base: Base{
				ClientCreator: testClients{client: client},
				AppName:       "policy-bot",
				PullOpts:      &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
				ConfigFetcher: &ConfigFetcher{Loader: mockConfigLoader{
					loadConfig: func(_ context.Context, _ *github.Client, owner, repo, ref string) (appconfig.Config, error) {
						assert.Equal(t, "original", owner)
						assert.Equal(t, "service", repo)
						assert.Equal(t, "release", ref)
						return appconfig.Config{Content: []byte(extendingPolicy), Source: "original/service@release", Path: ".policy.yml"}, nil
					},
					loadRemoteRef: func(context.Context, *github.Client, string, appconfig.RemoteRef) (appconfig.Config, error) {
						return appconfig.Config{Content: []byte(test.parent), Source: "shared/policies@main", Path: "parent.yml"}, test.loadError
					},
				}},
			}}
			payload := []byte(`{"action":"checks_requested","installation":{"id":1},"repository":{"name":"service","owner":{"login":"original"}},"merge_group":{"base_ref":"refs/heads/release","head_sha":"merge-head"}}`)
			require.NoError(t, h.Handle(context.Background(), "merge_group", "delivery", payload))
			require.Len(t, statuses, 1)
			status := <-statuses
			assert.Equal(t, "success", status.GetState())
			assert.Equal(t, "policy-bot: release", status.GetContext())
		})
	}
}
