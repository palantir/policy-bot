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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stackedPolicy = `
policy:
  approval:
    - ci passed

approval_rules:
  - name: ci passed
    if:
      has_status:
        statuses:
          - lint
    requires:
      count: 0
`

const (
	stackOwner  = "testorg"
	stackRepo   = "testrepo"
	stackRepoID = 1234
)

// stackSHA returns the head commit of the i-th pull request in a stack.
func stackSHA(i int) string {
	return fmt.Sprintf("%040d", i)
}

// fakeStack is a fake GitHub API for a stack of pull requests where pull
// request i targets pull request i-1 and so contains the head commits of all
// pull requests below it.
type fakeStack struct {
	size int

	mu        sync.Mutex
	requests  int
	evaluated map[int]bool
	statuses  map[string]int
}

func newFakeStack(size int) *fakeStack {
	return &fakeStack{
		size:      size,
		evaluated: make(map[int]bool),
		statuses:  make(map[string]int),
	}
}

// containing returns the pull requests that contain the head commit of pull
// request i, which is how GitHub populates commit-based event payloads.
func (s *fakeStack) containing(i int) []*github.PullRequest {
	var prs []*github.PullRequest
	for n := i; n <= s.size; n++ {
		prs = append(prs, &github.PullRequest{
			Number: new(n),
			State:  new("open"),
			Head:   &github.PullRequestBranch{SHA: new(stackSHA(n)), Ref: new(fmt.Sprintf("stack-%d", n))},
			Base: &github.PullRequestBranch{
				Ref:  new("main"),
				Repo: &github.Repository{ID: new(int64(stackRepoID))},
			},
		})
	}
	return prs
}

var (
	contentsPath   = regexp.MustCompile(`^/repos/[^/]+/[^/]+/contents/`)
	combinedPath   = regexp.MustCompile(`^/repos/[^/]+/[^/]+/commits/([0-9a-f]+)/status$`)
	checkRunsPath  = regexp.MustCompile(`^/repos/[^/]+/[^/]+/commits/([0-9a-f]+)/check-runs$`)
	commitPRsPath  = regexp.MustCompile(`^/repos/[^/]+/[^/]+/commits/([0-9a-f]+)/pulls$`)
	postStatusPath = regexp.MustCompile(`^/repos/[^/]+/[^/]+/statuses/([0-9a-f]+)$`)
)

func (s *fakeStack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++

	w.Header().Set("Content-Type", "application/json")
	write := func(v any) {
		_ = json.NewEncoder(w).Encode(v)
	}

	switch {
	case r.URL.Path == "/graphql":
		var body struct {
			Variables struct {
				Number int `json:"number"`
			} `json:"variables"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		n := body.Variables.Number
		s.evaluated[n] = true
		write(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
			"author":            map[string]any{"login": "author"},
			"title":             fmt.Sprintf("Stack %d", n),
			"createdAt":         "2026-01-01T00:00:00Z",
			"state":             "OPEN",
			"isCrossRepository": false,
			"isDraft":           false,
			"headRefOID":        stackSHA(n),
			"headRefName":       fmt.Sprintf("stack-%d", n),
			"headRepository":    map[string]any{"name": stackRepo, "owner": map[string]any{"login": stackOwner}},
			"baseRefName":       "main",
			"baseRepository":    map[string]any{"databaseId": stackRepoID},
			"changedFiles":      1,
		}}}})

	case contentsPath.MatchString(r.URL.Path):
		write(map[string]any{
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte(stackedPolicy)),
		})

	case combinedPath.MatchString(r.URL.Path):
		write(map[string]any{"statuses": []any{map[string]any{"context": "lint", "state": "success"}}})

	case checkRunsPath.MatchString(r.URL.Path):
		write(map[string]any{"total_count": 0, "check_runs": []any{}})

	case commitPRsPath.MatchString(r.URL.Path):
		var i int
		_, _ = fmt.Sscanf(commitPRsPath.FindStringSubmatch(r.URL.Path)[1], "%d", &i)
		write(s.containing(i))

	case r.Method == http.MethodPost && postStatusPath.MatchString(r.URL.Path):
		s.statuses[postStatusPath.FindStringSubmatch(r.URL.Path)[1]]++
		write(map[string]any{})

	default:
		w.WriteHeader(http.StatusNotFound)
		write(map[string]any{"message": "Not Found"})
	}
}

type fakeClientCreator struct {
	githubapp.ClientCreator
	url string
}

func (cc *fakeClientCreator) NewInstallationClient(int64) (*github.Client, error) {
	baseURL := cc.url + "/"
	return github.NewClient(github.WithURLs(&baseURL, nil))
}

func (cc *fakeClientCreator) NewInstallationV4Client(int64) (*githubv4.Client, error) {
	return githubv4.NewEnterpriseClient(cc.url+"/graphql", http.DefaultClient), nil
}

func newStackBase(t *testing.T, stack *fakeStack) Base {
	srv := httptest.NewServer(stack)
	t.Cleanup(srv.Close)

	return Base{
		ClientCreator: &fakeClientCreator{url: srv.URL},
		BaseConfig:    &baseapp.HTTPConfig{PublicURL: "https://policy-bot.example.com"},
		PullOpts:      &PullEvaluationOptions{StatusCheckContext: "policy-bot"},
		ConfigFetcher: &ConfigFetcher{
			Loader:          appconfig.NewLoader([]string{".policy.yml"}),
			SeenPolicyCache: NewSeenPolicyCache(),
		},
		AppName: "policy-bot",
	}
}

func stackRepository() *github.Repository {
	return &github.Repository{
		ID:    new(int64(stackRepoID)),
		Name:  new(stackRepo),
		Owner: &github.User{Login: new(stackOwner)},
	}
}

func marshalEvent(t *testing.T, event any) []byte {
	b, err := json.Marshal(event)
	require.NoError(t, err)
	return b
}

func checkRunEvent(t *testing.T, stack *fakeStack, i int) []byte {
	return marshalEvent(t, github.CheckRunEvent{
		Action: new("completed"),
		CheckRun: &github.CheckRun{
			Name:         new("lint"),
			HeadSHA:      new(stackSHA(i)),
			Conclusion:   new("success"),
			PullRequests: stack.containing(i),
		},
		Repo:         stackRepository(),
		Installation: &github.Installation{ID: new(int64(1))},
	})
}

func workflowRunEvent(t *testing.T, stack *fakeStack, i int) []byte {
	return marshalEvent(t, github.WorkflowRunEvent{
		Action: new("completed"),
		WorkflowRun: &github.WorkflowRun{
			HeadSHA:      new(stackSHA(i)),
			PullRequests: stack.containing(i),
		},
		Repo:         stackRepository(),
		Installation: &github.Installation{ID: new(int64(1))},
	})
}

func statusEvent(t *testing.T, i int) []byte {
	return marshalEvent(t, github.StatusEvent{
		Context:      new("lint"),
		State:        new("success"),
		Commit:       &github.RepositoryCommit{SHA: new(stackSHA(i))},
		Repo:         stackRepository(),
		Installation: &github.Installation{ID: new(int64(1))},
	})
}

func TestCommitEventsOnlyEvaluateHeadPullRequest(t *testing.T) {
	const size = 10
	const mid = 4

	tests := map[string]struct {
		eventType string
		handler   func(Base) githubapp.EventHandler
		payload   func(*testing.T, *fakeStack) []byte
	}{
		"check_run": {
			eventType: "check_run",
			handler:   func(b Base) githubapp.EventHandler { return &CheckRun{Base: b} },
			payload:   func(t *testing.T, s *fakeStack) []byte { return checkRunEvent(t, s, mid) },
		},
		"workflow_run": {
			eventType: "workflow_run",
			handler:   func(b Base) githubapp.EventHandler { return &WorkflowRun{Base: b} },
			payload:   func(t *testing.T, s *fakeStack) []byte { return workflowRunEvent(t, s, mid) },
		},
		"status": {
			eventType: "status",
			handler:   func(b Base) githubapp.EventHandler { return &Status{Base: b} },
			payload:   func(t *testing.T, s *fakeStack) []byte { return statusEvent(t, mid) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stack := newFakeStack(size)
			h := test.handler(newStackBase(t, stack))

			err := h.Handle(context.Background(), test.eventType, "delivery", test.payload(t, stack))
			require.NoError(t, err)

			assert.Equal(t, map[int]bool{mid: true}, stack.evaluated, "only the pull request whose head is the event commit should be evaluated")
			assert.Equal(t, map[string]int{stackSHA(mid): 1}, stack.statuses, "only the event commit should get a status")
		})
	}
}

func TestCommitEventsEvaluatePullRequestsWithoutHeadSHA(t *testing.T) {
	stack := newFakeStack(3)
	h := &CheckRun{Base: newStackBase(t, stack)}

	prs := stack.containing(1)
	for _, pr := range prs {
		pr.Head = nil
	}
	payload := marshalEvent(t, github.CheckRunEvent{
		Action: new("completed"),
		CheckRun: &github.CheckRun{
			Name:         new("lint"),
			HeadSHA:      new(stackSHA(1)),
			Conclusion:   new("success"),
			PullRequests: prs,
		},
		Repo:         stackRepository(),
		Installation: &github.Installation{ID: new(int64(1))},
	})

	require.NoError(t, h.Handle(context.Background(), "check_run", "delivery", payload))
	assert.Equal(t, map[int]bool{1: true, 2: true, 3: true}, stack.evaluated, "pull requests without a head SHA should still be evaluated")
}

// TestCheckRunRestackRequests counts the GitHub API requests made when every
// pull request in a stack gets a new head commit and a successful check, like
// after a restack. Before skipping pull requests whose head is a different
// commit, the work grew with the square of the stack size.
func TestCheckRunRestackRequests(t *testing.T) {
	for _, size := range []int{10, 25} {
		t.Run(fmt.Sprintf("stack of %d", size), func(t *testing.T) {
			stack := newFakeStack(size)
			h := &CheckRun{Base: newStackBase(t, stack)}

			for i := 1; i <= size; i++ {
				require.NoError(t, h.Handle(context.Background(), "check_run", "delivery", checkRunEvent(t, stack, i)))
			}

			statuses := 0
			for sha, n := range stack.statuses {
				assert.Equal(t, 1, n, "commit %s should get exactly one status", sha)
				statuses += n
			}
			assert.Equal(t, size, statuses)
			t.Logf("%d pull requests: %d API requests, %d statuses posted", size, stack.requests, statuses)
		})
	}
}
