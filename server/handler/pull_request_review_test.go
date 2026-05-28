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
	"net/url"
	"testing"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequestReviewSkipsWithoutHydratingPullContext(t *testing.T) {
	transport := &recordingTransport{
		configBody: `
policy:
  approval:
  - approval required

approval_rules:
- name: approval required
  requires:
    count: 1
`,
	}
	httpClient := &http.Client{Transport: transport}

	client := github.NewClient(httpClient)
	baseURL, err := url.Parse("http://github.localhost/")
	require.NoError(t, err)
	client.BaseURL = baseURL

	v4client := githubv4.NewClient(httpClient)

	h := PullRequestReview{
		Base: Base{
			ClientCreator: staticClientCreator{
				client:   client,
				v4client: v4client,
			},
			ConfigFetcher: &ConfigFetcher{
				Loader: appconfig.NewLoader([]string{".policy.yml"}),
			},
			BaseConfig: &baseapp.HTTPConfig{
				PublicURL: "https://policy-bot.example.com",
			},
			PullOpts: &PullEvaluationOptions{},
			AppName:  "policy-bot",
		},
	}

	pr := testPullRequest()
	pr.ChangedFiles = nil
	event := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		Review: &github.PullRequestReview{
			State: github.Ptr("commented"),
			Body:  github.Ptr("just a comment"),
			User: &github.User{
				Login: github.Ptr("reviewer"),
			},
		},
		PullRequest: pr,
		Repo:        pr.GetBase().GetRepo(),
		Sender: &github.User{
			Login: github.Ptr("reviewer"),
		},
		Installation: &github.Installation{
			ID: github.Ptr(int64(123)),
		},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	err = h.Handle(context.Background(), "pull_request_review", "delivery-id", payload)
	require.NoError(t, err)

	assert.Equal(t, 1, transport.configRequestCount())
	assert.Empty(t, transport.unexpectedRequests())
}
