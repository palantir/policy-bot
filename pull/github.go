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

func (ghc *GitHubContext) IsCollaborator(org, repo, user, desiredPerm string) (bool, error) {
	return ghc.mbrCtx.IsCollaborator(org, repo, user, desiredPerm)
}

func (ghc *GitHubContext) Locator() string {
	return fmt.Sprintf("%s/%s#%d", ghc.owner, ghc.repo, ghc.number)
}

func (ghc *GitHubContext) RepositoryOwner() string {
	return ghc.owner
}

func (ghc *GitHubContext) RepositoryName() string {
	return ghc.repo
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
		if err := ghc.loadPullRequestData(); err != nil {
			return nil, err
		}
	}

	if len(ghc.commits) >= MaxPullRequestCommits {
		return nil, errors.Errorf("too many commits in pull request, maximum is %d", MaxPullRequestCommits)
	}

	// verify that the head of the PR being evaluated exists in commit list
	// some GitHub APIs have a delay propagating commit information
	headSHA := ghc.pr.GetHead().GetSHA()
	for i := len(ghc.commits) - 1; i >= 0; i-- {
		if headSHA == ghc.commits[i].SHA {
			return ghc.commits, nil
		}
	}

	return nil, errors.Errorf("pull request head %s was missing from commit listing", headSHA)
}

func (ghc *GitHubContext) Comments() ([]*Comment, error) {
	if ghc.comments == nil {
		if err := ghc.loadPullRequestData(); err != nil {
			return nil, err
		}
	}
	return ghc.comments, nil
}

func (ghc *GitHubContext) Reviews() ([]*Review, error) {
	if ghc.reviews == nil {
		if err := ghc.loadPullRequestData(); err != nil {
			return nil, err
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
		backfillPushedDate(commits)

		ghc.targetCommits = make([]*Commit, len(commits))
		for i, c := range commits {
			ghc.targetCommits[i] = c.ToCommit()
		}
	}
	return ghc.targetCommits, nil
}

func (ghc *GitHubContext) loadPullRequestData() error {
	// do not query changed files here because they are only need for rules
	// that use file predicates, while comments, commits, and reviews are
	// needed for almost all rule evaluations
	var q struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					PageInfo v4PageInfo
					Nodes    []*v4IssueComment
				} `graphql:"comments(first: 100, after: $commentCursor)"`

				Commits struct {
					PageInfo v4PageInfo
					Nodes    []*v4PullRequestCommit
				} `graphql:"commits(first: 100, after: $commitCursor)"`

				Reviews struct {
					PageInfo v4PageInfo
					Nodes    []*v4PullRequestReview
				} `graphql:"reviews(first: 100, after: $reviewCursor, states: [APPROVED, CHANGES_REQUESTED])"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.owner),
		"name":   githubv4.String(ghc.repo),
		"number": githubv4.Int(ghc.number),

		"commentCursor": (*githubv4.String)(nil),
		"commitCursor":  (*githubv4.String)(nil),
		"reviewCursor":  (*githubv4.String)(nil),
	}

	// accumulate raw commits for post-processing
	var commits []*v4Commit
	ghc.reviews = make([]*Review, 0)
	ghc.comments = make([]*Comment, 0)

	for {
		complete := 0
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return errors.Wrap(err, "failed to load pull request data")
		}

		for _, c := range q.Repository.PullRequest.Comments.Nodes {
			ghc.comments = append(ghc.comments, c.ToComment())
		}
		if !q.Repository.PullRequest.Comments.PageInfo.UpdateCursor(qvars, "commentCursor") {
			complete++
		}

		for _, c := range q.Repository.PullRequest.Commits.Nodes {
			commits = append(commits, &c.Commit)
		}
		if !q.Repository.PullRequest.Commits.PageInfo.UpdateCursor(qvars, "commitCursor") {
			complete++
		}

		for _, r := range q.Repository.PullRequest.Reviews.Nodes {
			ghc.reviews = append(ghc.reviews, r.ToReview())
		}
		if !q.Repository.PullRequest.Reviews.PageInfo.UpdateCursor(qvars, "reviewCursor") {
			complete++
		}

		if complete == 3 {
			break
		}
	}

	backfillPushedDate(commits)

	ghc.commits = make([]*Commit, len(commits))
	for i, c := range commits {
		ghc.commits[i] = c.ToCommit()
	}

	return nil
}

type v4PageInfo struct {
	EndCursor   *githubv4.String
	HasNextPage bool
}

// UpdateCursor modifies the named cursor value in the the query variable map
// and returns true if there are additional pages.
func (pi v4PageInfo) UpdateCursor(vars map[string]interface{}, name string) bool {
	if pi.HasNextPage {
		vars[name] = githubv4.NewString(*pi.EndCursor)
		return true
	}

	// if this was the last page, set cursor so the next response is empty
	// on all queuries after that, the end cursor will be nil
	if pi.EndCursor != nil {
		vars[name] = githubv4.NewString(*pi.EndCursor)
	}
	return false
}

type v4PullRequestReview struct {
	Author      v4Actor
	State       string
	Body        string
	SubmittedAt time.Time
}

func (r *v4PullRequestReview) ToReview() *Review {
	return &Review{
		CreatedAt: r.SubmittedAt,
		Author:    r.Author.GetV3Login(),
		State:     ReviewState(strings.ToLower(r.State)),
		Body:      r.Body,
	}
}

type v4IssueComment struct {
	Author    v4Actor
	Body      string
	CreatedAt time.Time
}

func (c *v4IssueComment) ToComment() *Comment {
	return &Comment{
		CreatedAt: c.CreatedAt,
		Author:    c.Author.GetV3Login(),
		Body:      c.Body,
	}
}

type v4PullRequestCommit struct {
	Commit v4Commit
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
		Author:          c.Author.GetV3Login(),
		Committer:       c.Committer.GetV3Login(),
	}
}

// backfillPushedDate copies the push date from the HEAD commit in a batch push
// to all other commits in that batch. It assumes the commits slice is in
// ascending chronologic order (latest commit at the end).
func backfillPushedDate(commits []*v4Commit) {
	var lastPushed *time.Time
	for i := len(commits) - 1; i >= 0; i-- {
		if commits[i].PushedDate != nil {
			lastPushed = commits[i].PushedDate
		} else {
			commits[i].PushedDate = lastPushed
		}
	}
}

type v4Actor struct {
	Type  string `graphql:"__typename"`
	Login string
}

// GetV3Login returns a V3-compatible login string. These login strings contain
// the "[bot]" suffix for GitHub identities.
func (a v4Actor) GetV3Login() string {
	if a.Type == "Bot" {
		return a.Login + "[bot]"
	}
	return a.Login
}

type v4GitActor struct {
	User *v4Actor
}

func (ga v4GitActor) GetV3Login() string {
	if ga.User != nil {
		return ga.User.GetV3Login()
	}
	return ""
}

func isNotFound(err error) bool {
	if rerr, ok := err.(*github.ErrorResponse); ok {
		return rerr.Response.StatusCode == http.StatusNotFound
	}
	return false
}
