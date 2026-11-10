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

func (b *Base) completionPullRequests(ctx context.Context, installationID int64, repo *github.Repository, sha string, prs []*github.PullRequest) ([]*github.PullRequest, error) {
	if len(prs) > 0 || sha == "" {
		return prs, nil
	}
	client, err := b.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}
	return pullRequestsForHead(ctx, client, repo, sha)
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
