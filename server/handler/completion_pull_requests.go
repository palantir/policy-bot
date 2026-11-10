// Copyright 2018 Palantir Technologies, Inc.
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
	"sync"
	"time"

	"github.com/google/go-github/v93/github"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

const (
	completionPRPositiveTTL = 30 * time.Second
	completionPRNegativeTTL = 5 * time.Second
)

// CompletionPRCache bounds lookup traffic across check and workflow events.
// It caches associations only; policy evaluations still load current PR state.
type CompletionPRCache struct {
	mu      sync.Mutex
	entries *lru.Cache
	pending map[completionKey]*completionLookup
}

type completionKey struct {
	installationID int64
	repoID         int64
	sha            string
}

type completionLookup struct {
	done    chan struct{}
	prs     []*github.PullRequest
	err     error
	expires time.Time
}

func NewCompletionPRCache() *CompletionPRCache {
	entries, _ := lru.New(1024)
	return &CompletionPRCache{entries: entries, pending: make(map[completionKey]*completionLookup)}
}

func (c *CompletionPRCache) lookup(ctx context.Context, key completionKey, load func() ([]*github.PullRequest, error)) ([]*github.PullRequest, error) {
	if c == nil {
		return load()
	}
	c.mu.Lock()
	if value, ok := c.entries.Get(key); ok {
		entry := value.(*completionLookup)
		if time.Now().Before(entry.expires) {
			c.mu.Unlock()
			return entry.prs, entry.err
		}
		c.entries.Remove(key)
	}
	if pending, ok := c.pending[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pending.done:
			return pending.prs, pending.err
		}
	}
	result := &completionLookup{done: make(chan struct{})}
	c.pending[key] = result
	c.mu.Unlock()

	result.prs, result.err = load()
	c.mu.Lock()
	delete(c.pending, key)
	if result.err == nil {
		ttl := completionPRPositiveTTL
		if len(result.prs) == 0 {
			// Keep negative caching short because GitHub's commit index can lag.
			ttl = completionPRNegativeTTL
		}
		result.expires = time.Now().Add(ttl)
		c.entries.Add(key, result)
	}
	close(result.done)
	c.mu.Unlock()
	return result.prs, result.err
}

func (b *Base) completionPullRequests(ctx context.Context, installationID int64, repo *github.Repository, sha string, prs []*github.PullRequest) ([]*github.PullRequest, error) {
	if len(prs) > 0 || sha == "" {
		return prs, nil
	}
	return b.CompletionPRCache.lookup(ctx, completionKey{installationID, repo.GetID(), sha}, func() ([]*github.PullRequest, error) {
		client, err := b.NewInstallationClient(installationID)
		if err != nil {
			return nil, err
		}
		return pullRequestsForHead(ctx, client, repo, sha)
	})
}

func pullRequestsForHead(ctx context.Context, client *github.Client, repo *github.Repository, sha string) ([]*github.PullRequest, error) {
	// Fork completion events can omit associated PRs. Resolve them by commit,
	// excluding closed PRs and PRs that only contain this commit in their history.
	opts := &github.ListOptions{PerPage: 100}
	var matches []*github.PullRequest
	for {
		prs, response, err := client.PullRequests.ListPullRequestsWithCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), sha, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list pull requests for SHA %s", sha)
		}
		for _, pr := range prs {
			if pr.GetState() == "open" && pr.GetHead().GetSHA() == sha && pr.GetBase().GetRepo().GetID() == repo.GetID() {
				matches = append(matches, pr)
			}
		}
		if response.NextPage == 0 {
			return matches, nil
		}
		opts.Page = response.NextPage
	}
}
