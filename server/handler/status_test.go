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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusEvaluationLogsIncludePullRequestNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/commits/sha/pulls", r.URL.Path)
		_, err := w.Write([]byte(`[{"number":1,"state":"open"},{"number":2,"state":"closed"},{"number":3,"state":"open"}]`))
		assert.NoError(t, err)
	}))
	defer server.Close()
	client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(new(server.URL+"/"), nil))
	require.NoError(t, err)

	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	h := Status{Base: Base{ClientCreator: statusLogClientCreator{client: client}}}
	err = h.processOthers(logger.WithContext(context.Background()), github.StatusEvent{
		Repo:    &github.Repository{Name: new("repo"), Owner: &github.User{Login: new("owner")}},
		Commit:  &github.RepositoryCommit{SHA: new("sha")},
		Context: new("ci"),
	})
	require.EqualError(t, err, "failed to evaluate 2 pull requests")

	decoder := json.NewDecoder(&logs)
	var numbers []int
	for decoder.More() {
		var entry map[string]interface{}
		require.NoError(t, decoder.Decode(&entry))
		if entry["level"] == "error" {
			require.Contains(t, entry, githubapp.LogKeyPRNum)
			numbers = append(numbers, int(entry[githubapp.LogKeyPRNum].(float64)))
			assert.Equal(t, "owner", entry[githubapp.LogKeyRepositoryOwner])
			assert.Equal(t, "repo", entry[githubapp.LogKeyRepositoryName])
		}
	}
	assert.Equal(t, []int{1, 3}, numbers)
}

type statusLogClientCreator struct {
	githubapp.ClientCreator
	client *github.Client
}

func (c statusLogClientCreator) NewInstallationClient(int64) (*github.Client, error) {
	return c.client, nil
}

func (c statusLogClientCreator) NewInstallationV4Client(int64) (*githubv4.Client, error) {
	return nil, errors.New("evaluation unavailable")
}
