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

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"
)

const (
	// MaxPullRequestFiles is the max number of files returned by GitHub
	// https://developer.github.com/v3/pulls/#list-pull-requests-files
	MaxPullRequestFiles = 3000

	// MaxPullRequestCommits is the max number of commits returned by GitHub
	// https://developer.github.com/v3/pulls/#list-commits-on-a-pull-request
	MaxPullRequestCommits = 250
)

// Locator identifies a pull request and optionally contains a full or partial
// pull request object.
type Locator struct {
	Owner  string
	Repo   string
	Number int

	Value *github.PullRequest
}

type TemporaryError struct {
	error string
}

func (te *TemporaryError) Error() string {
	return te.error
}

// IsComplete returns true if the locator contains a pull request object with
// all required fields.
func (loc Locator) IsComplete() bool {
	switch {
	case loc.Value == nil:
	case loc.Value.Draft == nil:
	case loc.Value.GetTitle() == "":
	case loc.Value.GetCreatedAt().IsZero():
	case loc.Value.GetState() == "":
	case loc.Value.GetUser().GetLogin() == "":
	case loc.Value.GetBase().GetRef() == "":
	case loc.Value.GetBase().GetRepo().GetID() == 0:
	case loc.Value.GetHead().GetSHA() == "":
	case loc.Value.GetHead().GetRef() == "":
	case loc.Value.GetHead().GetRepo().GetID() == 0:
	case loc.Value.GetHead().GetRepo().GetName() == "":
	case loc.Value.GetHead().GetRepo().GetOwner().GetLogin() == "":
	default:
		return true
	}
	return false
}

// toV4 returns a v4PullRequest, loading data from the API if the locator is not complete.
func (loc Locator) toV4(ctx context.Context, client *githubv4.Client) (*v4PullRequest, error) {
	if !loc.IsComplete() {
		var q struct {
			Repository struct {
				PullRequest v4PullRequest `graphql:"pullRequest(number: $number)"`
			} `graphql:"repository(owner: $owner, name: $name)"`
		}
		qvars := map[string]interface{}{
			"owner":  githubv4.String(loc.Owner),
			"name":   githubv4.String(loc.Repo),
			"number": githubv4.Int(loc.Number),
		}
		if err := client.Query(ctx, &q, qvars); err != nil {
			return nil, errors.Wrap(err, "failed to load pull request details")
		}
		return &q.Repository.PullRequest, nil
	}

	var v4 v4PullRequest
	v4.Title = loc.Value.GetTitle()
	v4.Author.Login = loc.Value.GetUser().GetLogin()
	v4.CreatedAt = loc.Value.GetCreatedAt()
	v4.State = loc.Value.GetState()
	v4.IsCrossRepository = loc.Value.GetHead().GetRepo().GetID() != loc.Value.GetBase().GetRepo().GetID()
	v4.HeadRefOID = loc.Value.GetHead().GetSHA()
	v4.HeadRefName = loc.Value.GetHead().GetRef()
	v4.HeadRepository.Name = loc.Value.GetHead().GetRepo().GetName()
	v4.HeadRepository.Owner.Login = loc.Value.GetHead().GetRepo().GetOwner().GetLogin()
	v4.BaseRefName = loc.Value.GetBase().GetRef()
	v4.IsDraft = loc.Value.GetDraft()
	return &v4, nil
}

// GitHubContext is a Context implementation that gets information from GitHub.
// A new instance must be created for each request.
type GitHubContext struct {
	MembershipContext

	ctx      context.Context
	client   *github.Client
	v4client *githubv4.Client

	owner  string
	repo   string
	number int
	pr     *v4PullRequest

	// cached fields
	files         []*File
	commits       []*Commit
	comments      []*Comment
	reviews       []*Review
	reviewers     []*Reviewer
	collaborators map[string]string
	teams         map[string]string
	teamIDs       map[string]int64
	membership    map[string]bool
	statuses      map[string]string
	labels        []string
}

// NewGitHubContext creates a new pull.Context that makes GitHub requests to
// obtain information. It caches responses for the lifetime of the context. The
// pull request passed to the context must contain at least the base repository
// and the number or the function panics.
func NewGitHubContext(ctx context.Context, mbrCtx MembershipContext, client *github.Client, v4client *githubv4.Client, loc Locator) (Context, error) {
	if loc.Owner == "" || loc.Repo == "" || loc.Number == 0 {
		panic("pull request object does not contain full identifying information")
	}

	pr, err := loc.toV4(ctx, v4client)
	if err != nil {
		return nil, err
	}

	return &GitHubContext{
		MembershipContext: mbrCtx,

		ctx:      ctx,
		client:   client,
		v4client: v4client,

		owner:  loc.Owner,
		repo:   loc.Repo,
		number: loc.Number,
		pr:     pr,
	}, nil
}

func (ghc *GitHubContext) RepositoryOwner() string {
	return ghc.owner
}

func (ghc *GitHubContext) RepositoryName() string {
	return ghc.repo
}

func (ghc *GitHubContext) Number() int {
	return ghc.number
}

func (ghc *GitHubContext) Title() string {
	return ghc.pr.Title
}

func (ghc *GitHubContext) Author() string {
	return ghc.pr.Author.GetV3Login()
}

func (ghc *GitHubContext) CreatedAt() time.Time {
	return ghc.pr.CreatedAt
}

func (ghc *GitHubContext) IsOpen() bool {
	return ghc.pr.State == "open"
}

func (ghc *GitHubContext) IsClosed() bool {
	return ghc.pr.State == "closed"
}

func (ghc *GitHubContext) HeadSHA() string {
	return ghc.pr.HeadRefOID
}

func (ghc *GitHubContext) IsDraft() bool {
	return ghc.pr.IsDraft
}

// Branches returns the names of the base and head branch. If the head branch
// is from another repository (it is a fork) then the branch name is
// `owner:branchName`.
func (ghc *GitHubContext) Branches() (base string, head string) {
	base = ghc.pr.BaseRefName
	head = ghc.pr.HeadRefName
	if ghc.pr.IsCrossRepository {
		head = ghc.pr.HeadRepository.Owner.Login + ":" + head
	}
	return
}

func (ghc *GitHubContext) ChangedFiles() ([]*File, error) {
	if ghc.files == nil {
		opt := github.ListOptions{
			PerPage: 100,
		}

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

		ghc.files = make([]*File, 0, len(allFiles))
		for _, f := range allFiles {
			status := FileModified
			switch f.GetStatus() {
			case "added":
				status = FileAdded
			case "deleted":
				status = FileDeleted
			case "renamed":
				// Break renames into components: the new file is added and we
				// generate an extra entry for the old file that is deleted.
				// Attribute all modifications to the new file to avoid double
				// counting.
				status = FileAdded
				ghc.files = append(ghc.files, &File{
					Filename:  f.GetPreviousFilename(),
					Status:    FileDeleted,
					Additions: 0,
					Deletions: 0,
				})
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
		commits, err := ghc.loadCommits()
		if err != nil {
			return nil, err
		}
		if len(commits) >= MaxPullRequestCommits {
			return nil, errors.Errorf("too many commits in pull request, maximum is %d", MaxPullRequestCommits)
		}

		backfillPushedAt(commits, ghc.pr.HeadRefOID)
		ghc.commits = commits
	}
	return ghc.commits, nil
}

func (ghc *GitHubContext) Comments() ([]*Comment, error) {
	if ghc.comments == nil {
		if err := ghc.loadPagedData(); err != nil {
			return nil, err
		}
	}
	return ghc.comments, nil
}

func (ghc *GitHubContext) Reviews() ([]*Review, error) {
	if ghc.reviews == nil {
		if err := ghc.loadPagedData(); err != nil {
			return nil, err
		}
	}
	return ghc.reviews, nil
}

func (ghc *GitHubContext) RepositoryCollaborators() (map[string]string, error) {
	if ghc.collaborators == nil {
		opts := github.ListCollaboratorsOptions{
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}

		collaborators := make(map[string]string)
		for {
			users, res, err := ghc.client.Repositories.ListCollaborators(ghc.ctx, ghc.owner, ghc.repo, &opts)
			if err != nil {
				return nil, errors.Wrap(err, "failed to list repository collaborators")
			}
			for _, u := range users {
				collaborators[u.GetLogin()] = coalescePermission(u.GetPermissions())
			}
			if res.NextPage == 0 {
				break
			}
			opts.Page = res.NextPage
		}
		ghc.collaborators = collaborators
	}
	return ghc.collaborators, nil
}

func coalescePermission(perms map[string]bool) string {
	// standardize to new-style values used by GraphQL and other endpoints
	switch {
	case perms["admin"]:
		return "admin"
	case perms["push"] || perms["write"]:
		return "write"
	case perms["pull"] || perms["read"]:
		return "read"
	}
	return "none"
}

func (ghc *GitHubContext) RequestedReviewers() ([]*Reviewer, error) {
	if ghc.reviewers == nil {
		if err := ghc.loadRequestedReviewers(); err != nil {
			return nil, err
		}
	}
	return ghc.reviewers, nil
}

func (ghc *GitHubContext) loadRequestedReviewers() error {
	var q struct {
		Repository struct {
			PullRequest struct {
				ReviewRequests struct {
					PageInfo v4PageInfo
					Nodes    []struct {
						RequestedReviewer v4RequestedReviewer
					}
				} `graphql:"reviewRequests(first: 100, after: $requestCursor)"`

				TimelineItems struct {
					PageInfo v4PageInfo
					Nodes    []struct {
						ReviewRequestRemovedEvent struct {
							Actor             v4Actor
							RequestedReviewer v4RequestedReviewer
						} `graphql:"... on ReviewRequestRemovedEvent"`
					}
				} `graphql:"timelineItems(first: 100, after: $timelineCursor, itemTypes: [REVIEW_REQUEST_REMOVED_EVENT])"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.owner),
		"name":   githubv4.String(ghc.repo),
		"number": githubv4.Int(ghc.number),

		"requestCursor":  (*githubv4.String)(nil),
		"timelineCursor": (*githubv4.String)(nil),
	}

	reviewers := []*Reviewer{}
	for {
		complete := 0
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return errors.Wrap(err, "failed to load requested reviewers data")
		}

		for _, n := range q.Repository.PullRequest.ReviewRequests.Nodes {
			reviewers = append(reviewers, n.RequestedReviewer.ToReviewer(false))
		}
		if !q.Repository.PullRequest.ReviewRequests.PageInfo.UpdateCursor(qvars, "reviewCursor") {
			complete++
		}

		for _, n := range q.Repository.PullRequest.TimelineItems.Nodes {
			reviewers = append(reviewers, n.ReviewRequestRemovedEvent.RequestedReviewer.ToReviewer(true))
		}
		if !q.Repository.PullRequest.TimelineItems.PageInfo.UpdateCursor(qvars, "timelineCursor") {
			complete++
		}

		if complete == 2 {
			break
		}
	}
	ghc.reviewers = reviewers
	return nil
}

func (ghc *GitHubContext) Teams() (map[string]string, error) {
	if ghc.teams == nil {
		opt := &github.ListOptions{
			PerPage: 100,
		}

		allTeams := make(map[string]string)
		for {
			teams, resp, err := ghc.client.Repositories.ListTeams(ghc.ctx, ghc.RepositoryOwner(), ghc.RepositoryName(), opt)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to list teams page %d", opt.Page)
			}
			for _, t := range teams {
				allTeams[t.GetSlug()] = t.GetPermission()
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		ghc.teams = allTeams
	}
	return ghc.teams, nil
}

func (ghc *GitHubContext) LatestStatuses() (map[string]string, error) {
	if ghc.statuses == nil {
		statuses, err := ghc.getStatuses()
		if err != nil {
			return nil, err
		}

		checkStatuses, err := ghc.getCheckStatuses()
		if err != nil {
			return nil, err
		}

		for k, v := range checkStatuses {
			statuses[k] = v
		}

		ghc.statuses = statuses
	}

	return ghc.statuses, nil
}

func (ghc *GitHubContext) getStatuses() (map[string]string, error) {
	opt := &github.ListOptions{
		PerPage: 100,
	}
	// get all pages of results
	statuses := make(map[string]string)
	for {
		combinedStatus, resp, err := ghc.client.Repositories.GetCombinedStatus(ghc.ctx, ghc.owner, ghc.repo, ghc.HeadSHA(), opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get statuses for page %d", opt.Page)
		}
		for _, s := range combinedStatus.Statuses {
			statuses[s.GetContext()] = s.GetState()
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return statuses, nil
}

func (ghc *GitHubContext) getCheckStatuses() (map[string]string, error) {
	opt := &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	// get all pages of results
	statuses := make(map[string]string)
	for {
		checkRuns, resp, err := ghc.client.Checks.ListCheckRunsForRef(ghc.ctx, ghc.owner, ghc.repo, ghc.HeadSHA(), opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get check runs for page %d", opt.Page)
		}
		for _, checkRun := range checkRuns.CheckRuns {
			statuses[checkRun.GetName()] = checkRun.GetConclusion()
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return statuses, nil
}

func (ghc *GitHubContext) Labels() ([]string, error) {
	if ghc.labels == nil {
		issueLabels, _, err := ghc.client.Issues.ListLabelsByIssue(ghc.ctx, ghc.owner, ghc.repo, ghc.number, &github.ListOptions{
			Page:    0,
			PerPage: 100,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list labels")
		}

		labels := make([]string, len(issueLabels))
		for i, label := range issueLabels {
			labels[i] = strings.ToLower(label.GetName())
		}
		ghc.labels = labels
	}
	return ghc.labels, nil
}

func (ghc *GitHubContext) loadPagedData() error {
	// this is a minor optimization: make max(c,r) requests instead of c+r
	var q struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					PageInfo v4PageInfo
					Nodes    []v4IssueComment
				} `graphql:"comments(first: 100, after: $commentCursor)"`

				Reviews struct {
					PageInfo v4PageInfo
					Nodes    []v4PullRequestReview
				} `graphql:"reviews(first: 100, after: $reviewCursor, states: [APPROVED, CHANGES_REQUESTED, COMMENTED])"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.owner),
		"name":   githubv4.String(ghc.repo),
		"number": githubv4.Int(ghc.number),

		"commentCursor": (*githubv4.String)(nil),
		"reviewCursor":  (*githubv4.String)(nil),
	}

	comments := []*Comment{}
	reviews := []*Review{}
	for {
		complete := 0
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return errors.Wrap(err, "failed to load pull request data")
		}

		for _, c := range q.Repository.PullRequest.Comments.Nodes {
			comments = append(comments, c.ToComment())
		}
		if !q.Repository.PullRequest.Comments.PageInfo.UpdateCursor(qvars, "commentCursor") {
			complete++
		}

		for _, r := range q.Repository.PullRequest.Reviews.Nodes {
			switch r.State {
			case "COMMENTED":
				comments = append(comments, r.ToComment())
			case "APPROVED", "CHANGES_REQUESTED":
				reviews = append(reviews, r.ToReview())
			}
		}
		if !q.Repository.PullRequest.Reviews.PageInfo.UpdateCursor(qvars, "reviewCursor") {
			complete++
		}

		if complete == 2 {
			break
		}
	}

	ghc.comments = comments
	ghc.reviews = reviews
	return nil
}

func (ghc *GitHubContext) loadCommits() ([]*Commit, error) {
	log := zerolog.Ctx(ghc.ctx)

	rawCommits, err := ghc.loadRawCommits()
	if err != nil {
		return nil, err
	}

	var head *Commit
	commits := make([]*Commit, 0, len(rawCommits))

	for _, r := range rawCommits {
		c := r.Commit.ToCommit()
		if c.SHA == ghc.pr.HeadRefOID {
			head = c
		}
		commits = append(commits, c)
	}

	// fail early if head is missing from the pull request
	if head == nil {
		return nil, errors.Errorf("head commit %.10s is missing, probably due to a force-push", ghc.pr.HeadRefOID)
	}

	// As of 2020-02-05, the pushed data may be missing when loaded via the
	// pull request APIs if:
	//
	//  - the commit comes from a fork (always missing in this case)
	//  - the data has not propagated yet
	//
	// In the second case, retrying after a delay can fix things, but the delay
	// can be 15+ seconds in practice, so using the alternate API should
	// improve latency at the cost of more API requests.
	if head.PushedAt == nil {
		log.Debug().
			Bool("fork", ghc.pr.IsCrossRepository).
			Msgf("failed to load pushed date via pull request, falling back to commit APIs")

		if err := ghc.loadPushedAt(commits); err != nil {
			return nil, err
		}
	}

	if head.PushedAt == nil {
		return nil, errors.Errorf("head commit %.10s is missing pushed date; this is probably a bug", ghc.pr.HeadRefOID)
	}
	return commits, nil
}

func (ghc *GitHubContext) loadRawCommits() ([]*v4PullRequestCommit, error) {
	var q struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					PageInfo v4PageInfo
					Nodes    []*v4PullRequestCommit
				} `graphql:"commits(first: 100, after: $cursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.owner),
		"name":   githubv4.String(ghc.repo),
		"number": githubv4.Int(ghc.number),
		"cursor": (*githubv4.String)(nil),
	}

	commits := []*v4PullRequestCommit{}
	for {
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return nil, errors.Wrap(err, "failed to load commits")
		}
		commits = append(commits, q.Repository.PullRequest.Commits.Nodes...)
		if !q.Repository.PullRequest.Commits.PageInfo.UpdateCursor(qvars, "cursor") {
			break
		}
	}
	return commits, nil
}

func (ghc *GitHubContext) loadPushedAt(commits []*Commit) error {
	commitsBySHA := make(map[string]*Commit, len(commits))
	for _, c := range commits {
		commitsBySHA[c.SHA] = c
	}

	var q struct {
		Repository struct {
			Object struct {
				Commit struct {
					History struct {
						PageInfo v4PageInfo
						Nodes    []struct {
							OID        string
							PushedDate *time.Time
						}
					} `graphql:"history(first: 100, after: $cursor)"`
				} `graphql:"... on Commit"`
			} `graphql:"object(oid: $oid)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	qvars := map[string]interface{}{
		"owner":  githubv4.String(ghc.pr.HeadRepository.Owner.Login),
		"name":   githubv4.String(ghc.pr.HeadRepository.Name),
		"oid":    githubv4.GitObjectID(ghc.pr.HeadRefOID),
		"cursor": (*githubv4.String)(nil),
	}

	loaded := 0
	for {
		if err := ghc.v4client.Query(ghc.ctx, &q, qvars); err != nil {
			return errors.Wrap(err, "failed to load commit pushed dates")
		}
		for _, n := range q.Repository.Object.Commit.History.Nodes {
			if c, ok := commitsBySHA[n.OID]; ok {
				c.PushedAt = n.PushedDate
				delete(commitsBySHA, n.OID)
			}
		}

		loaded += len(q.Repository.Object.Commit.History.Nodes)
		if loaded > len(commits) {
			break
		}

		if !q.Repository.Object.Commit.History.PageInfo.UpdateCursor(qvars, "cursor") {
			break
		}
	}

	if len(commitsBySHA) > 0 {
		missingSHAs := make([]string, 0, len(commitsBySHA))
		for sha := range commitsBySHA {
			missingSHAs = append(missingSHAs, sha)
		}

		err := &TemporaryError{fmt.Sprintf("%d commits were not found while loading pushed dates. Missing %s.",
			len(commitsBySHA), strings.Join(missingSHAs, ", "))}
		return err
	}
	return nil
}

func backfillPushedAt(commits []*Commit, headSHA string) {
	commitsBySHA := make(map[string]*Commit, len(commits))
	for _, c := range commits {
		commitsBySHA[c.SHA] = c
	}

	root := headSHA
	for {
		c, ok := commitsBySHA[root]
		if !ok || len(c.Parents) == 0 {
			break
		}

		firstParent, ok := commitsBySHA[c.Parents[0]]
		if !ok {
			break
		}

		if firstParent.PushedAt == nil {
			firstParent.PushedAt = c.PushedAt
		}

		delete(commitsBySHA, root)
		root = firstParent.SHA
	}
}

// if adding new fields to this struct, modify Locator#toV4() and Locator#IsComplete() as well
type v4PullRequest struct {
	Author    v4Actor
	Title     string
	CreatedAt time.Time
	State     string

	IsCrossRepository bool

	// This field is in GraphQL Preview, so don't ask for it just yet
	IsDraft bool `graphql:""`

	HeadRefOID     string
	HeadRefName    string
	HeadRepository struct {
		Name  string
		Owner v4Actor
	}

	BaseRefName string
}

type v4PageInfo struct {
	EndCursor   *githubv4.String
	HasNextPage bool
}

// UpdateCursor modifies the named cursor value in the the query variable map
// and returns true if there are additional pages.
func (pi v4PageInfo) UpdateCursor(vars map[string]interface{}, name string) bool {
	if pi.HasNextPage && pi.EndCursor != nil {
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

func (r *v4PullRequestReview) ToComment() *Comment {
	return &Comment{
		CreatedAt: r.SubmittedAt,
		Author:    r.Author.GetV3Login(),
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
	Author          v4GitActor
	Committer       v4GitActor
	CommittedViaWeb bool
	PushedDate      *time.Time
	Parents         struct {
		Nodes []struct {
			OID string
		}
	} `graphql:"parents(first: 3)"`
	Signature *v4GitSignature
}

func (c *v4Commit) ToCommit() *Commit {
	var parents []string
	for _, p := range c.Parents.Nodes {
		parents = append(parents, p.OID)
	}

	var signature *Signature
	if c.Signature != nil {
		signature = c.Signature.ToSignature()
	}

	return &Commit{
		SHA:             c.OID,
		Parents:         parents,
		CommittedViaWeb: c.CommittedViaWeb,
		Author:          c.Author.GetV3Login(),
		Committer:       c.Committer.GetV3Login(),
		PushedAt:        c.PushedDate,
		Signature:       signature,
	}
}

type v4RequestedReviewer struct {
	User v4Actor `graphql:"... on User"`
	Team v4Team  `graphql:"... on Team"`
}

func (rr *v4RequestedReviewer) ToReviewer(removed bool) *Reviewer {
	r := Reviewer{
		Removed: removed,
	}
	switch {
	case rr.User.Login != "":
		r.Type = ReviewerUser
		r.Name = rr.User.GetV3Login()
	case rr.Team.Slug != "":
		r.Type = ReviewerTeam
		r.Name = rr.Team.Slug
	}
	return &r
}

type v4Team struct {
	Slug string
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

type SignatureType string

const (
	SignatureGpg   SignatureType = "GpgSignature"
	SignatureSmime SignatureType = "SmimeSignature"
)

type v4GitSignature struct {
	Type  string           `graphql:"__typename"`
	GPG   v4GpgSignature   `graphql:"... on GpgSignature"`
	SMIME v4SmimeSignature `graphql:"... on SmimeSignature"`
}

func (s *v4GitSignature) ToSignature() *Signature {
	switch SignatureType(s.Type) {
	case SignatureGpg:
		return &Signature{
			IsValid: s.GPG.IsValid,
			KeyID:   s.GPG.KeyID,
			Signer:  s.GPG.Signer.GetV3Login(),
			State:   s.GPG.State,
			Type:    SignatureGpg,
		}
	case SignatureSmime:
		return &Signature{
			IsValid: s.SMIME.IsValid,
			Signer:  s.SMIME.Signer.GetV3Login(),
			State:   s.SMIME.State,
			Type:    SignatureSmime,
		}
	default:
		return nil
	}
}

type v4SmimeSignature struct {
	Email             string
	IsValid           bool
	Payload           string
	Signature         string
	Signer            *v4Actor
	State             string
	WasSignedByGitHub bool
}

type v4GpgSignature struct {
	Email             string
	IsValid           bool
	KeyID             string
	Payload           string
	Signature         string
	Signer            *v4Actor
	State             string
	WasSignedByGitHub bool
}
