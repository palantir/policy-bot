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
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/pull"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestNewCheckRunOptions(t *testing.T) {
	tests := map[string]struct {
		state              string
		expectedStatus     string
		expectedConclusion string
		hasCompletedAt     bool
	}{
		"pending_maps_to_in_progress": {
			state:              "pending",
			expectedStatus:     "in_progress",
			expectedConclusion: "",
			hasCompletedAt:     false,
		},
		"success_maps_to_completed_success": {
			state:              "success",
			expectedStatus:     "completed",
			expectedConclusion: "success",
			hasCompletedAt:     true,
		},
		"failure_maps_to_completed_failure": {
			state:              "failure",
			expectedStatus:     "completed",
			expectedConclusion: "failure",
			hasCompletedAt:     true,
		},
		"error_maps_to_completed_action_required": {
			state:              "error",
			expectedStatus:     "completed",
			expectedConclusion: "action_required",
			hasCompletedAt:     true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			detailsURL := "https://example.com/details"
			opts := NewCheckRunOptions("policy-bot: main", "abc123", test.state, "test message", &detailsURL)

			assert.Equal(t, "policy-bot: main", opts.Name)
			assert.Equal(t, "abc123", opts.HeadSHA)
			assert.Equal(t, test.expectedStatus, opts.GetStatus())
			assert.Equal(t, test.expectedConclusion, opts.GetConclusion())

			assert.NotNil(t, opts.StartedAt, "StartedAt should always be set")

			if test.hasCompletedAt {
				assert.NotNil(t, opts.CompletedAt, "CompletedAt should be set for completed states")
			} else {
				assert.Nil(t, opts.CompletedAt, "CompletedAt should not be set for in_progress")
			}

			require.NotNil(t, opts.Output)
			assert.Equal(t, "test message", opts.Output.GetTitle())
			assert.Equal(t, "test message", opts.Output.GetSummary())
			assert.Equal(t, "https://example.com/details", opts.GetDetailsURL())
		})
	}
}

func TestNewCheckRunOptions_NilDetailsURL(t *testing.T) {
	opts := NewCheckRunOptions("policy-bot: main", "abc123", "success", "installed", nil)

	assert.Nil(t, opts.DetailsURL)
	assert.Equal(t, "policy-bot: main", opts.Name)
	assert.Equal(t, "completed", opts.GetStatus())
	assert.Equal(t, "success", opts.GetConclusion())
}

func TestNewEvalContextDoesNotPrefetchPullData(t *testing.T) {
	transport := &recordingTransport{
		configDelay: 100 * time.Millisecond,
	}
	httpClient := &http.Client{Transport: transport}

	client := github.NewClient(httpClient)
	baseURL, err := url.Parse("http://github.localhost/")
	require.NoError(t, err)
	client.BaseURL = baseURL

	v4client := githubv4.NewClient(httpClient)

	h := Base{
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
	}

	pr := testPullRequest()
	_, err = h.NewEvalContext(context.Background(), 123, pull.Locator{
		Owner:  pr.GetBase().GetRepo().GetOwner().GetLogin(),
		Repo:   pr.GetBase().GetRepo().GetName(),
		Number: pr.GetNumber(),
		Value:  pr,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, transport.configRequestCount())
	assert.Empty(t, transport.unexpectedRequests())
}

type staticClientCreator struct {
	client   *github.Client
	v4client *githubv4.Client
}

func (c staticClientCreator) NewAppClient() (*github.Client, error) {
	return c.client, nil
}

func (c staticClientCreator) NewAppV4Client() (*githubv4.Client, error) {
	return c.v4client, nil
}

func (c staticClientCreator) NewInstallationClient(_ int64) (*github.Client, error) {
	return c.client, nil
}

func (c staticClientCreator) NewInstallationV4Client(_ int64) (*githubv4.Client, error) {
	return c.v4client, nil
}

func (c staticClientCreator) NewTokenSourceClient(_ oauth2.TokenSource) (*github.Client, error) {
	return c.client, nil
}

func (c staticClientCreator) NewTokenSourceV4Client(_ oauth2.TokenSource) (*githubv4.Client, error) {
	return c.v4client, nil
}

func (c staticClientCreator) NewTokenClient(_ string) (*github.Client, error) {
	return c.client, nil
}

func (c staticClientCreator) NewTokenV4Client(_ string) (*githubv4.Client, error) {
	return c.v4client, nil
}

type recordingTransport struct {
	mu             sync.Mutex
	configRequests int
	unexpected     []string
	configDelay    time.Duration
	configBody     string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/repos/testorg/testrepo/contents/.policy.yml" {
		time.Sleep(rt.configDelay)
		rt.mu.Lock()
		rt.configRequests++
		rt.mu.Unlock()
		body := rt.configBody
		if body == "" {
			body = "{}\n"
		}
		content := base64.StdEncoding.EncodeToString([]byte(body))
		return jsonResponse(req, http.StatusOK, `{"type":"file","encoding":"base64","content":"`+content+`","name":".policy.yml","path":".policy.yml"}`), nil
	}

	rt.mu.Lock()
	rt.unexpected = append(rt.unexpected, req.Method+" "+req.URL.Path)
	rt.mu.Unlock()

	switch req.URL.Path {
	case "/repos/testorg/testrepo/pulls/123/files":
		return jsonResponse(req, http.StatusOK, `[]`), nil
	case "/repos/testorg/testrepo/actions/runs":
		return jsonResponse(req, http.StatusOK, `{"total_count":0,"workflow_runs":[]}`), nil
	case "/graphql":
		return jsonResponse(req, http.StatusOK, `{"data":{}}`), nil
	default:
		return jsonResponse(req, http.StatusNotFound, `{}`), nil
	}
}

func (rt *recordingTransport) configRequestCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.configRequests
}

func (rt *recordingTransport) unexpectedRequests() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.unexpected...)
}

func jsonResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func testPullRequest() *github.PullRequest {
	return &github.PullRequest{
		Title:     github.Ptr("test title"),
		State:     github.Ptr("open"),
		Number:    github.Ptr(123),
		CreatedAt: &github.Timestamp{Time: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)},
		Draft:     github.Ptr(false),
		User: &github.User{
			Login: github.Ptr("reviewer"),
		},
		Head: &github.PullRequestBranch{
			Ref: github.Ptr("feature/test"),
			SHA: github.Ptr("head-sha"),
			Repo: &github.Repository{
				ID: github.Ptr(int64(1)),
				Owner: &github.User{
					Login: github.Ptr("testorg"),
				},
				Name: github.Ptr("testrepo"),
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.Ptr("main"),
			SHA: github.Ptr("base-sha"),
			Repo: &github.Repository{
				ID: github.Ptr(int64(1)),
				Owner: &github.User{
					Login: github.Ptr("testorg"),
				},
				Name: github.Ptr("testrepo"),
			},
		},
		ChangedFiles: github.Ptr(1),
	}
}
