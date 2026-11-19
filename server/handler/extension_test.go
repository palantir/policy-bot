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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/stretchr/testify/require"
)

type handlerTransport http.HandlerFunc

func (h handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	response := httptest.NewRecorder()
	h(response, req)
	result := response.Result()
	result.Request = req
	return result, nil
}

type testClients struct {
	githubapp.ClientCreator
	client *github.Client
}

func (c testClients) NewInstallationClient(int64) (*github.Client, error) {
	return c.client, nil
}

const extensionPolicy = `policy: {approval_extends: {remote: example/shared, path: .policy.yml, ref: main}}`

func extensionTestClient(t *testing.T, handler http.HandlerFunc) *github.Client {
	t.Helper()
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: handlerTransport(handler)}))
	require.NoError(t, err)
	return client
}
