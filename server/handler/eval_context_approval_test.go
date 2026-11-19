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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBotLogin = "policy-bot[bot]"

// fakeReviewAPI is a minimal GitHub API that serves a fixed list of reviews
// and records the review and status calls policy-bot makes.
type fakeReviewAPI struct {
	reviews []*github.PullRequestReview

	mu         sync.Mutex
	created    []github.PullRequestReviewRequest
	dismissed  []int64
	statuses   []string
	listCalled bool
}

func (f *fakeReviewAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/repos/testorg/testrepo/statuses/"):
		var s github.RepoStatus
		_ = json.NewDecoder(r.Body).Decode(&s)
		f.statuses = append(f.statuses, s.GetState())
		_, _ = io.WriteString(w, "{}")

	case r.Method == http.MethodGet && path == "/repos/testorg/testrepo/pulls/42/reviews":
		f.listCalled = true
		_ = json.NewEncoder(w).Encode(f.reviews)

	case r.Method == http.MethodPost && path == "/repos/testorg/testrepo/pulls/42/reviews":
		var req github.PullRequestReviewRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = append(f.created, req)
		_, _ = io.WriteString(w, "{}")

	case r.Method == http.MethodPut && strings.HasSuffix(path, "/dismissals"):
		var id int64
		_, _ = fmt.Sscanf(path, "/repos/testorg/testrepo/pulls/42/reviews/%d/dismissals", &id)
		f.dismissed = append(f.dismissed, id)
		_, _ = io.WriteString(w, "{}")

	default:
		http.Error(w, "unexpected request: "+r.Method+" "+path, http.StatusNotFound)
	}
}

func review(id int64, login, state, sha string) *github.PullRequestReview {
	return &github.PullRequestReview{
		ID:       github.Ptr(id),
		User:     &github.User{Login: github.Ptr(login)},
		State:    github.Ptr(state),
		CommitID: github.Ptr(sha),
	}
}

func newApprovalEvalContext(t *testing.T, api *fakeReviewAPI, enabled bool, prState string) *EvalContext {
	t.Helper()

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	client, err := github.NewClient(
		github.WithHTTPClient(srv.Client()),
		github.WithURLs(github.Ptr(srv.URL+"/"), nil),
	)
	require.NoError(t, err)

	return &EvalContext{
		Client: client,
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
			PostApprovalReview: enabled,
		},
		PublicURL: "https://policy-bot.example.com",
		AppName:   "policy-bot",
		PullContext: &pulltest.Context{
			OwnerValue:     "testorg",
			RepoValue:      "testrepo",
			NumberValue:    42,
			StateValue:     prState,
			HeadSHAValue:   "head123",
			BranchBaseName: "main",
		},
	}
}

func TestPostStatusSuccessSubmitsApproval(t *testing.T) {
	api := &fakeReviewAPI{}
	ec := newApprovalEvalContext(t, api, true, "open")

	ec.PostStatus(context.Background(), "success", "Approved by alice")

	assert.Equal(t, []string{"success"}, api.statuses)
	require.Len(t, api.created, 1)
	assert.Equal(t, "APPROVE", api.created[0].GetEvent())
	assert.Equal(t, "head123", api.created[0].GetCommitID())
	assert.Contains(t, api.created[0].GetBody(), "Approved by alice")
	assert.Empty(t, api.dismissed)
}

func TestPostStatusSuccessSkipsExistingApprovalOnHead(t *testing.T) {
	api := &fakeReviewAPI{reviews: []*github.PullRequestReview{
		review(1, testBotLogin, "APPROVED", "head123"),
	}}
	ec := newApprovalEvalContext(t, api, true, "open")

	ec.PostStatus(context.Background(), "success", "Approved by alice")

	assert.Empty(t, api.created)
	assert.Empty(t, api.dismissed)
}

func TestPostStatusSuccessApprovesNewHeadCommit(t *testing.T) {
	api := &fakeReviewAPI{reviews: []*github.PullRequestReview{
		review(1, testBotLogin, "APPROVED", "old456"),
	}}
	ec := newApprovalEvalContext(t, api, true, "open")

	ec.PostStatus(context.Background(), "success", "Approved by alice")

	require.Len(t, api.created, 1)
	assert.Equal(t, "head123", api.created[0].GetCommitID())
}

func TestPostStatusNotSuccessDismissesOnlyBotApprovals(t *testing.T) {
	for _, state := range []string{"pending", "failure", "error"} {
		t.Run(state, func(t *testing.T) {
			api := &fakeReviewAPI{reviews: []*github.PullRequestReview{
				review(1, testBotLogin, "APPROVED", "old456"),
				review(2, testBotLogin, "APPROVED", "head123"),
				review(3, testBotLogin, "DISMISSED", "older789"),
				review(4, "alice", "APPROVED", "head123"),
			}}
			ec := newApprovalEvalContext(t, api, true, "open")

			ec.PostStatus(context.Background(), state, "0/1 rules approved")

			assert.ElementsMatch(t, []int64{1, 2}, api.dismissed)
			assert.Empty(t, api.created)
		})
	}
}

func TestPostStatusDoesNotTouchReviewsWhenDisabled(t *testing.T) {
	api := &fakeReviewAPI{reviews: []*github.PullRequestReview{
		review(1, testBotLogin, "APPROVED", "head123"),
	}}
	ec := newApprovalEvalContext(t, api, false, "open")

	ec.PostStatus(context.Background(), "success", "Approved by alice")
	ec.PostStatus(context.Background(), "pending", "0/1 rules approved")

	assert.Equal(t, []string{"success", "pending"}, api.statuses)
	assert.False(t, api.listCalled)
	assert.Empty(t, api.created)
	assert.Empty(t, api.dismissed)
}

func TestPostStatusDoesNotTouchReviewsWhenSkippingStatus(t *testing.T) {
	api := &fakeReviewAPI{}
	ec := newApprovalEvalContext(t, api, true, "open")
	ec.SkipPostStatus = true

	ec.PostStatus(context.Background(), "success", "Approved by alice")

	assert.False(t, api.listCalled)
	assert.Empty(t, api.created)
}

func TestPostStatusDoesNotTouchReviewsOnClosedPullRequest(t *testing.T) {
	api := &fakeReviewAPI{}
	ec := newApprovalEvalContext(t, api, true, "closed")

	ec.PostStatus(context.Background(), "success", "Approved by alice")

	assert.False(t, api.listCalled)
	assert.Empty(t, api.created)
}

func TestExcludeReviewsByHidesBotReviews(t *testing.T) {
	prctx := excludeReviewsBy(&pulltest.Context{
		ReviewsValue: []*pull.Review{
			{ID: "1", Author: testBotLogin, State: pull.ReviewApproved},
			{ID: "2", Author: "alice", State: pull.ReviewApproved},
		},
	}, testBotLogin)

	reviews, err := prctx.Reviews()
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, "alice", reviews[0].Author)
}
