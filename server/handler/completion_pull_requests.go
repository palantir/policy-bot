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

	"github.com/google/go-github/v93/github"
	"github.com/pkg/errors"
)

func pullRequestsForHead(ctx context.Context, client *github.Client, repo *github.Repository, sha string) ([]*github.PullRequest, error) {
	// Fork completion events can omit associated PRs. Resolve them by commit,
	// excluding closed PRs and PRs that only contain this commit in their history.
	prs, err := pullRequestsForCommit(ctx, client, repo, sha)
	if err != nil {
		return nil, err
	}
	var matches []*github.PullRequest
	for _, pr := range prs {
		if pr.GetState() == "open" && pr.GetHead().GetSHA() == sha && pr.GetBase().GetRepo().GetID() == repo.GetID() {
			matches = append(matches, pr)
		}
	}
	return matches, nil
}

// pullRequestsForCommit retrieves up to 100 associations, including historical commits.
// Callers apply the filtering appropriate to their event type.
func pullRequestsForCommit(ctx context.Context, client *github.Client, repo *github.Repository, sha string) ([]*github.PullRequest, error) {
	prs, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), sha, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list pull requests for SHA %s", sha)
	}
	return prs, nil
}
