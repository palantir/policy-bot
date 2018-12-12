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
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
)

const (
	// MaxPullRequestFiles is the max number of files returned by GitHub
	// https://developer.github.com/v3/pulls/#list-pull-requests-files
	MaxPullRequestFiles = 300

	// MaxPullRequestCommits is the max number of commits returned by GitHub
	// https://developer.github.com/v3/pulls/#list-commits-on-a-pull-request
	MaxPullRequestCommits = 250

	// TargetCommitLimit is the max number of commits to retrieve from the
	// target branch of a pull request. 100 items is the maximum that can be
	// retrieved from the GitHub API without paging.
	TargetCommitLimit = 100
)

// GitHubContext is a Context implementation that gets information from GitHub.
// A new instance must be created for each request.
type GitHubContext struct {
	ctx      context.Context
	client   *github.Client
	v4client *githubv4.Client
	mbrCtx   MembershipContext

	owner  string
	repo   string
	number int
	pr     *github.PullRequest

	// cached fields
	files         []*File
	commits       []*Commit
	targetCommits []*Commit
	comments      []*Comment
	reviews       []*Review
	teamIDs       map[string]int64
	membership    map[string]bool
}

func NewGitHubContext(ctx context.Context, mbrCtx MembershipContext, client *github.Client, v4client *githubv4.Client, pr *github.PullRequest) Context {
	return &GitHubContext{
		ctx:      ctx,
		client:   client,
		v4client: v4client,
		mbrCtx:   mbrCtx,

		owner:  pr.GetBase().GetRepo().GetOwner().GetLogin(),
		repo:   pr.GetBase().GetRepo().GetName(),
		number: pr.GetNumber(),
		pr:     pr,
	}
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
	if len(ghc.files) >= MaxPullRequestFiles {
		return nil, errors.Errorf("too many files in pull request, maximum is %d", MaxPullRequestFiles)
	}
	return ghc.files, nil
}

func (ghc *GitHubContext) Commits() ([]*Commit, error) {
	if ghc.commits == nil {
		if err := ghc.loadTimeline(); err != nil {
			return nil, errors.Wrap(err, "failed to fetch commits")
		}
	}
	if len(ghc.commits) >= MaxPullRequestCommits {
		return nil, errors.Errorf("too many commits in pull request, maximum is %d", MaxPullRequestCommits)
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
	base = ghc.pr.GetBase().GetRef()

	if ghc.pr.GetHead().GetRepo().GetID() == ghc.pr.GetBase().GetRepo().GetID() {
		head = ghc.pr.GetHead().GetRef()
	} else {
		head = ghc.pr.GetHead().GetLabel()
	}

	return
}

func (ghc *GitHubContext) TargetCommits() ([]*Commit, error) {
	if ghc.targetCommits == nil {
		var q struct {
			Repository struct {
				Ref struct {
					Target struct {
						Commit struct {
							History struct {
								Nodes []*v4Commit
							} `graphql:"history(first: $limit)"`
						} `graphql:"... on Commit"`
					}
				} `graphql:"ref(qualifiedName: $ref)"`
			} `graphql:"repository(owner: $owner, name: $name)"`
		}
		qvars := map[string]interface{}{
			"owner": githubv4.String(ghc.owner),
			"name":  githubv4.String(ghc.repo),
			"ref":   githubv4.String(ghc.pr.GetBase().GetRef()),
			"limit": githubv4.Int(TargetCommitLimit),
		}

		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return nil, errors.Wrap(err, "failed to list target commits")
		}

		commits := q.Repository.Ref.Target.Commit.History.Nodes
		ghc.targetCommits = make([]*Commit, len(commits))

		// GitHub only records pushedDate for the HEAD commit in a batch push
		// Backfill this to all other commits for the purpose of sorting
		var lastPushed *time.Time
		for i := len(commits) - 1; i >= 0; i-- {
			if commits[i].PushedDate != nil {
				lastPushed = commits[i].PushedDate
			} else {
				commits[i].PushedDate = lastPushed
			}

			ghc.targetCommits[i] = commits[i].ToCommit()
		}
	}
	return ghc.targetCommits, nil
}

func (ghc *GitHubContext) loadTimeline() error {
	var q struct {
		Repository struct {
			PullRequest struct {
				Timeline struct {
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage bool
					}
					Nodes []*v4TimelineEvent
				} `graphql:"timeline(first: 100, after: $cursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.owner),
		"name":   githubv4.String(ghc.repo),
		"number": githubv4.Int(ghc.number),
		"cursor": (*githubv4.String)(nil),
	}

	var allEvents []*v4TimelineEvent
	for {
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return errors.Wrap(err, "failed to list pull request timeline")
		}

		allEvents = append(allEvents, q.Repository.PullRequest.Timeline.Nodes...)
		if !q.Repository.PullRequest.Timeline.PageInfo.HasNextPage {
			break
		}
		qvars["cursor"] = githubv4.NewString(q.Repository.PullRequest.Timeline.PageInfo.EndCursor)
	}

	// GitHub only records pushedDate for the HEAD commit in a batch push
	// Backfill this to all other commits for the purpose of sorting
	var lastPushed *time.Time
	for i := len(allEvents) - 1; i >= 0; i-- {
		if event := allEvents[i]; event.Type == "Commit" {
			if event.Commit.PushedDate != nil {
				lastPushed = event.Commit.PushedDate
			} else {
				event.Commit.PushedDate = lastPushed
			}
		}
	}

	// Order events by ascending creation time
	// Use stable sort to keep ordering for commits with the same timestamp
	sort.SliceStable(allEvents, func(i, j int) bool {
		return allEvents[i].CreatedAt().Before(allEvents[j].CreatedAt())
	})

	ghc.commits = make([]*Commit, 0)
	ghc.reviews = make([]*Review, 0)
	ghc.comments = make([]*Comment, 0)

	for _, event := range allEvents {
		switch event.Type {
		case "Commit":
			ghc.commits = append(ghc.commits, event.Commit.ToCommit())
		case "PullRequestReview":
			state := ReviewState(strings.ToLower(event.PullRequestReview.State))
			ghc.reviews = append(ghc.reviews, &Review{
				CreatedAt: event.CreatedAt(),
				Author:    event.PullRequestReview.Author.Login,
				State:     state,
				Body:      event.PullRequestReview.Body,
				ID:        event.PullRequestReview.ID,
			})
		case "ReviewDismissedEvent":
			for _, r := range ghc.reviews {
				if r.ID == event.ReviewDismissedEvent.Review.ID {
					r.State = ReviewDismissed
				}
			}
		case "IssueComment":
			ghc.comments = append(ghc.comments, &Comment{
				CreatedAt: event.CreatedAt(),
				Author:    event.IssueComment.Author.Login,
				Body:      event.IssueComment.Body,
			})
		}
	}

	return nil
}

type v4TimelineEvent struct {
	Type string `graphql:"__typename"`

	Commit v4Commit `graphql:"... on Commit"`

	PullRequestReview struct {
		ID          string
		Author      v4Actor
		State       string
		Body        string
		SubmittedAt time.Time
	} `graphql:"... on PullRequestReview"`

	ReviewDismissedEvent struct {
		CreatedAt time.Time
		Review    struct {
			ID string
		}
	} `graphql:"... on ReviewDismissedEvent"`

	IssueComment struct {
		Author    v4Actor
		Body      string
		CreatedAt time.Time
	} `graphql:"... on IssueComment"`
}

func (event *v4TimelineEvent) CreatedAt() (t time.Time) {
	switch event.Type {
	case "Commit":
		if event.Commit.PushedDate != nil {
			t = *event.Commit.PushedDate
		}
	case "PullRequestReview":
		t = event.PullRequestReview.SubmittedAt
	case "ReviewDismissedEvent":
		t = event.ReviewDismissedEvent.CreatedAt
	case "IssueComment":
		t = event.IssueComment.CreatedAt
	}
	return
}

type v4Commit struct {
	OID             string
	PushedDate      *time.Time
	Author          v4GitActor
	Committer       v4GitActor
	CommittedViaWeb bool
	Parents         struct {
		Nodes []struct {
			OID string
		}
	} `graphql:"parents(first: 10)"`
}

func (c *v4Commit) ToCommit() *Commit {
	var parents []string
	for _, p := range c.Parents.Nodes {
		parents = append(parents, p.OID)
	}

	var createdAt time.Time
	if c.PushedDate != nil {
		createdAt = *c.PushedDate
	}

	return &Commit{
		CreatedAt:       createdAt,
		SHA:             c.OID,
		Parents:         parents,
		CommittedViaWeb: c.CommittedViaWeb,
		Author:          c.Author.GetLogin(),
		Committer:       c.Committer.GetLogin(),
	}
}

type v4Actor struct {
	Login string
}

type v4GitActor struct {
	User *v4Actor
}

func (ga v4GitActor) GetLogin() string {
	if ga.User != nil {
		return ga.User.Login
	}
	return ""
}

func isNotFound(err error) bool {
	if rerr, ok := err.(*github.ErrorResponse); ok {
		return rerr.Response.StatusCode == http.StatusNotFound
	}
	return false
}
