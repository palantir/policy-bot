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

package pull

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

const (
	// MaxPullRequestFiles is the max number of files returned by GitHub
	// https://developer.github.com/v3/pulls/#list-pull-requests-files
	MaxPullRequestFiles = 300

	// MaxPullRequestCommits is the max number of commits returned by GitHub
	// https://developer.github.com/v3/pulls/#list-commits-on-a-pull-request
	MaxPullRequestCommits = 250
)

// GitHubContext is a Context implementation that gets information from GitHub.
// A new instance must be created for each request.
type GitHubContext struct {
	ctx    context.Context
	client *github.Client
	mbrCtx MembershipContext

	owner  string
	repo   string
	number int
	pr     *github.PullRequest

	// cached fields
	files             []*File
	commitsHaveAuthor bool
	commits           []*Commit
	comments          []*Comment
	reviews           []*Review
	teamIDs           map[string]int64
	membership        map[string]bool
}

func NewGitHubContext(ctx context.Context, mbrCtx MembershipContext, client *github.Client, pr *github.PullRequest) Context {
	return &GitHubContext{
		ctx:    ctx,
		client: client,
		mbrCtx: mbrCtx,

		owner:  pr.GetBase().GetRepo().GetOwner().GetLogin(),
		repo:   pr.GetBase().GetRepo().GetName(),
		number: pr.GetNumber(),
		pr:     pr,
	}
}

func (ghc *GitHubContext) ensurePRCached() error {
	if ghc.pr == nil {
		pr, _, err := ghc.client.PullRequests.Get(ghc.ctx, ghc.owner, ghc.repo, ghc.number)
		if err != nil {
			return errors.Wrap(err, "failed to get pull request")
		}
		ghc.pr = pr
	}

	return nil
}

func (ghc *GitHubContext) IsTeamMember(team, user string) (bool, error) {
	return ghc.mbrCtx.IsTeamMember(team, user)
}

func (ghc *GitHubContext) IsOrgMember(org, user string) (bool, error) {
	return ghc.mbrCtx.IsOrgMember(org, user)
}

func (ghc *GitHubContext) Locator() string {
	return fmt.Sprintf("%s/%s#%d", ghc.owner, ghc.repo, ghc.number)
}

func (ghc *GitHubContext) Author() (string, error) {
	if err := ghc.ensurePRCached(); err != nil {
		return "", errors.Wrap(err, "failed to get author")
	}

	return ghc.pr.GetUser().GetLogin(), nil
}

func (ghc *GitHubContext) ChangedFiles() ([]*File, error) {
	if ghc.files == nil {
		var opt github.ListOptions
		var allFiles []*github.CommitFile
		for {
			files, res, err := ghc.client.PullRequests.ListFiles(ghc.ctx, ghc.owner, ghc.repo, ghc.number, &opt)
			if err != nil {
				return nil, errors.Wrap(err, "failed to list pull request files")
			}
			allFiles = append(allFiles, files...)
			if res.NextPage == 0 {
				break
			}
			opt.Page = res.NextPage
		}

		if len(allFiles) >= MaxPullRequestFiles {
			return nil, errors.Errorf("too many files in pull request, maximum is %d", MaxPullRequestFiles)
		}

		for _, f := range allFiles {
			var status FileStatus
			switch f.GetStatus() {
			case "added":
				status = FileAdded
			case "deleted":
				status = FileDeleted
			case "modified":
				status = FileModified
			}

			ghc.files = append(ghc.files, &File{
				Filename:  f.GetFilename(),
				Status:    status,
				Additions: f.GetAdditions(),
				Deletions: f.GetDeletions(),
			})
		}
	}
	return ghc.files, nil
}

func (ghc *GitHubContext) Commits() ([]*Commit, error) {
	if !ghc.commitsHaveAuthor {
		if ghc.commits == nil {
			if err := ghc.loadTimeline(); err != nil {
				return nil, errors.Wrap(err, "failed to fetch commits")
			}
		}

		var opt github.ListOptions
		var allCommits []*github.RepositoryCommit
		for {
			commits, res, err := ghc.client.PullRequests.ListCommits(ghc.ctx, ghc.owner, ghc.repo, ghc.number, &opt)
			if err != nil {
				return nil, errors.Wrap(err, "failed to list pull request commits")
			}
			allCommits = append(allCommits, commits...)
			if res.NextPage == 0 {
				break
			}
			opt.Page = res.NextPage
		}

		if len(allCommits) >= MaxPullRequestCommits {
			return nil, errors.Errorf("too many commits in pull request, maximum is %d", MaxPullRequestCommits)
		}

		commitsBySha := make(map[string]*github.RepositoryCommit)
		for _, c := range allCommits {
			commitsBySha[c.GetSHA()] = c
		}

		for _, orderedCommit := range ghc.commits {
			commitWithLogin := commitsBySha[orderedCommit.SHA]
			orderedCommit.Committer = commitWithLogin.GetCommitter().GetLogin()
			orderedCommit.Author = commitWithLogin.GetAuthor().GetLogin()
		}

		ghc.commitsHaveAuthor = true
	}

	return ghc.commits, nil
}

func (ghc *GitHubContext) Comments() ([]*Comment, error) {
	if ghc.comments == nil {
		if err := ghc.loadTimeline(); err != nil {
			return nil, errors.Wrap(err, "failed to fetch comments")
		}
	}

	return ghc.comments, nil
}

func (ghc *GitHubContext) Reviews() ([]*Review, error) {
	if ghc.reviews == nil {
		if err := ghc.loadTimeline(); err != nil {
			return nil, errors.Wrap(err, "failed to fetch reviews")
		}
	}

	return ghc.reviews, nil
}

// Branches returns the names of the base and head branch. If the head branch is from another repository (it is a fork)
// then the branch name is `owner:branchName`.
func (ghc *GitHubContext) Branches() (base string, head string, err error) {
	if err := ghc.ensurePRCached(); err != nil {
		return "", "", errors.Wrap(err, "failed to get branches")
	}

	base = ghc.pr.GetBase().GetRef()

	if ghc.pr.GetHead().GetRepo().GetID() == ghc.pr.GetBase().GetRepo().GetID() {
		head = ghc.pr.GetHead().GetRef()
	} else {
		head = ghc.pr.GetHead().GetLabel()
	}

	return
}

func (ghc *GitHubContext) loadTimeline() error {
	var opts github.ListOptions
	var allEvents []*pullrequestEvent
	for {
		events, res, err := listIssueTimelineEvents(ghc.ctx, ghc.client, ghc.owner, ghc.repo, ghc.number, &opts)
		if err != nil {
			return errors.Wrap(err, "failed to list pull request timeline")
		}

		allEvents = append(allEvents, events...)
		if res.NextPage == 0 {
			break
		}

		opts.Page = res.NextPage
	}

	ghc.commits = make([]*Commit, 0)
	ghc.reviews = make([]*Review, 0)
	ghc.comments = make([]*Comment, 0)

	for i, event := range allEvents {
		switch event.GetEvent() {
		case "committed":
			ghc.commits = append(ghc.commits, &Commit{
				Order: i,
				SHA:   event.GetSHA(),
			})
		case "reviewed":
			state := ReviewState(strings.ToLower(event.GetState()))
			ghc.reviews = append(ghc.reviews, &Review{
				Order:        i,
				Author:       event.User.GetLogin(),
				LastModified: event.GetSubmittedAt(),
				State:        state,
				Body:         event.GetBody(),
			})
		case "commented":
			ghc.comments = append(ghc.comments, &Comment{
				Order:        i,
				Author:       event.GetActor().GetLogin(),
				LastModified: event.GetCreatedAt(),
				Body:         event.GetBody(),
			})
		}
	}

	return nil
}

func isNotFound(err error) bool {
	if rerr, ok := err.(*github.ErrorResponse); ok {
		return rerr.Response.StatusCode == http.StatusNotFound
	}
	return false
}
