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
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v90/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func repoClient(t *testing.T, body string) *github.Client {
	client, err := github.NewClient(
		github.WithTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "/repos/testorg/testrepo", req.URL.Path)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})),
	)
	require.NoError(t, err, "failed to create github client")
	return client
}

func TestPolicyRef(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		b := &Base{PullOpts: &PullEvaluationOptions{}}

		ref, err := b.policyRef(context.Background(), nil, "testorg", "testrepo", "feature-base", "main")
		require.NoError(t, err)
		assert.Equal(t, "feature-base", ref)
	})

	b := &Base{PullOpts: &PullEvaluationOptions{PolicyFromDefaultBranch: true}}

	t.Run("payloadDefaultBranch", func(t *testing.T) {
		ref, err := b.policyRef(context.Background(), nil, "testorg", "testrepo", "feature-base", "main")
		require.NoError(t, err)
		assert.Equal(t, "main", ref, "uses the payload value without an API call")
	})

	t.Run("apiFallback", func(t *testing.T) {
		client := repoClient(t, `{"id": 1, "default_branch": "develop"}`)

		ref, err := b.policyRef(context.Background(), client, "testorg", "testrepo", "feature-base", "")
		require.NoError(t, err)
		assert.Equal(t, "develop", ref)
	})

	t.Run("noDefaultBranch", func(t *testing.T) {
		client := repoClient(t, `{"id": 1}`)

		_, err := b.policyRef(context.Background(), client, "testorg", "testrepo", "feature-base", "")
		assert.Error(t, err)
	})
}
