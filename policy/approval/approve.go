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

package approval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Rule struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Predicates  predicate.Predicates `yaml:"if"`
	Options     Options              `yaml:"options"`
	Requires    common.Requires      `yaml:"requires"`
}

type Options struct {
	AllowAuthor               bool `yaml:"allow_author"`
	AllowContributor          bool `yaml:"allow_contributor"`
	AllowNonAuthorContributor bool `yaml:"allow_non_author_contributor"`
	InvalidateOnPush          bool `yaml:"invalidate_on_push"`

	IgnoreEditedComments bool          `yaml:"ignore_edited_comments"`
	IgnoreUpdateMerges   bool          `yaml:"ignore_update_merges"`
	IgnoreCommitsBy      common.Actors `yaml:"ignore_commits_by"`

	RequestReview RequestReview `yaml:"request_review"`

	Methods *common.Methods `yaml:"methods"`
}

type RequestReview struct {
	Enabled bool               `yaml:"enabled"`
	Mode    common.RequestMode `yaml:"mode"`
	Count   int                `yaml:"count"`
}

func (opts *Options) GetMethods() *common.Methods {
	methods := opts.Methods
	if methods == nil {
		methods = &common.Methods{}
	}
	if methods.Comments == nil {
		methods.Comments = []string{
			":+1:",
			"👍",
		}
	}
	if methods.GithubReview == nil {
		defaultGithubReview := true
		methods.GithubReview = &defaultGithubReview
	}

	methods.GithubReviewState = pull.ReviewApproved
	return methods
}

func (r *Rule) Trigger() common.Trigger {
	t := common.TriggerCommit

	if r.Requires.Count > 0 {
		m := r.Options.GetMethods()
		if len(m.Comments) > 0 || len(m.CommentPatterns) > 0 {
			t |= common.TriggerComment
		}
		if len(m.BodyPatterns) > 0 {
			t |= common.TriggerPullRequest
		}
		if m.GithubReview != nil && *m.GithubReview || len(m.GithubReviewCommentPatterns) > 0 {
			t |= common.TriggerReview
		}
	}

	for _, p := range r.Predicates.Predicates() {
		t |= p.Trigger()
	}

	return t
}

func (r *Rule) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	log := zerolog.Ctx(ctx)

	res.Name = r.Name
	res.Description = r.Description
	res.Status = common.StatusSkipped
	res.Requires = r.Requires
	res.Methods = r.Options.GetMethods()

	var predicateResults []*common.PredicateResult

	for _, p := range r.Predicates.Predicates() {
		result, err := p.Evaluate(ctx, prctx)
		if err != nil {
			res.Error = errors.Wrap(err, "failed to evaluate predicate")
			return
		}
		predicateResults = append(predicateResults, result)

		if !result.Satisfied {
			log.Debug().Msgf("skipping rule, predicate of type %T was not satisfied", p)

			desc := result.Description
			res.StatusDescription = desc
			if desc == "" {
				res.StatusDescription = "A precondition of this rule was not satisfied"
			}
			res.PredicateResults = []*common.PredicateResult{result}
			return
		}
	}
	res.PredicateResults = predicateResults

	candidates, dismissals, err := r.FilteredCandidates(ctx, prctx)
	if err != nil {
		res.Error = errors.Wrap(err, "failed to filter candidates")
		return
	}

	approved, approvers, err := r.IsApproved(ctx, prctx, candidates)
	if err != nil {
		res.Error = errors.Wrap(err, "failed to compute approval status")
		return
	}

	res.Approvers = approvers
	res.Dismissals = dismissals
	res.StatusDescription = r.statusDescription(approved, approvers, candidates)

	if approved {
		res.Status = common.StatusApproved
	} else {
		res.Status = common.StatusPending
		res.ReviewRequestRule = r.getReviewRequestRule()
	}

	return
}

func (r *Rule) getReviewRequestRule() *common.ReviewRequestRule {
	if !r.Options.RequestReview.Enabled {
		return nil
	}

	mode := r.Options.RequestReview.Mode
	if mode == "" {
		mode = common.RequestModeRandomUsers
	}

	requestedCount := r.Options.RequestReview.Count
	if requestedCount == 0 {
		requestedCount = r.Requires.Count
	}

	return &common.ReviewRequestRule{
		Users:          r.Requires.Actors.Users,
		Teams:          r.Requires.Actors.Teams,
		Organizations:  r.Requires.Actors.Organizations,
		Permissions:    r.Requires.Actors.GetPermissions(),
		RequiredCount:  r.Requires.Count,
		RequestedCount: requestedCount,
		Mode:           mode,
	}
}

func (r *Rule) IsApproved(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) (bool, []*common.Candidate, error) {
	log := zerolog.Ctx(ctx)

	if r.Requires.Count <= 0 {
		log.Debug().Msg("rule requires no approvals")
		return true, nil, nil
	}

	log.Debug().Msgf("found %d candidates for approval", len(candidates))

	// collect users "banned" by approval options
	banned := make(map[string]bool)

	// "author" is the user who opened the PR
	// if contributors are allowed, the author counts as a contributor
	author := prctx.Author()

	if !r.Options.AllowAuthor && !r.Options.AllowContributor {
		banned[author] = true
	}

	// "contributor" is any user who added a commit to the PR
	if !r.Options.AllowContributor && !r.Options.AllowNonAuthorContributor {
		commits, err := r.filteredCommits(ctx, prctx)
		if err != nil {
			return false, nil, err
		}

		for _, c := range commits {
			for _, u := range c.Users() {
				if u != author {
					banned[u] = true
				}
			}
		}
	}

	// filter real approvers using banned status and required membership
	var approvers []*common.Candidate
	for _, c := range candidates {
		if banned[c.User] {
			log.Debug().Str("user", c.User).Msg("rejecting approval by banned user")
			continue
		}

		isApprover, err := r.Requires.Actors.IsActor(ctx, prctx, c.User)
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to check candidate status")
		}
		if !isApprover {
			log.Debug().Str("user", c.User).Msg("ignoring approval by non-required user")
			continue
		}

		approvers = append(approvers, c)
	}

	log.Debug().Msgf("found %d/%d required approvers", len(approvers), r.Requires.Count)
	return len(approvers) >= r.Requires.Count, approvers, nil
}

// FilteredCandidates returns the potential approval candidates and any
// candidates that should be dimissed due to rule options.
func (r *Rule) FilteredCandidates(ctx context.Context, prctx pull.Context) ([]*common.Candidate, []*common.Dismissal, error) {

	candidates, err := r.Options.GetMethods().Candidates(ctx, prctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get approval candidates")
	}

	sort.Stable(common.CandidatesByCreationTime(candidates))

	var editDismissals []*common.Dismissal
	if r.Options.IgnoreEditedComments {
		candidates, editDismissals, err = r.filterEditedCandidates(ctx, prctx, candidates)
		if err != nil {
			return nil, nil, err
		}
	}

	var pushDismissals []*common.Dismissal
	if r.Options.InvalidateOnPush {
		candidates, pushDismissals, err = r.filterInvalidCandidates(ctx, prctx, candidates)
		if err != nil {
			return nil, nil, err
		}
	}

	var dismissals []*common.Dismissal
	dismissals = append(dismissals, editDismissals...)
	dismissals = append(dismissals, pushDismissals...)

	return candidates, dismissals, nil
}

func (r *Rule) filterEditedCandidates(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) ([]*common.Candidate, []*common.Dismissal, error) {
	log := zerolog.Ctx(ctx)

	if !r.Options.IgnoreEditedComments {
		return candidates, nil, nil
	}

	var allowed []*common.Candidate
	var dismissed []*common.Dismissal
	for _, c := range candidates {
		if c.LastEditedAt.IsZero() {
			allowed = append(allowed, c)
		} else {
			dismissed = append(dismissed, &common.Dismissal{
				Candidate: c,
				Reason:    "Comment was edited",
			})
		}
	}

	log.Debug().Msgf("discarded %d candidates with edited comments", len(dismissed))

	return allowed, dismissed, nil
}

func (r *Rule) filterInvalidCandidates(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) ([]*common.Candidate, []*common.Dismissal, error) {
	log := zerolog.Ctx(ctx)

	commits, err := r.filteredCommits(ctx, prctx)
	if err != nil {
		return nil, nil, err
	}
	if len(commits) == 0 {
		return candidates, nil, nil
	}

	sha := commits[0].SHA
	lastPushedAt, err := prctx.PushedAt(sha)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get last push timestamp")
	}

	var allowed []*common.Candidate
	var dismissed []*common.Dismissal
	for _, c := range candidates {
		if c.CreatedAt.After(lastPushedAt) {
			allowed = append(allowed, c)
		} else {
			dismissed = append(dismissed, &common.Dismissal{
				Candidate: c,
				Reason:    fmt.Sprintf("Invalidated by push of %.7s", sha),
			})
		}
	}

	log.Debug().Msgf(
		"discarded %d candidates invalidated by push of %s on or before %s",
		len(dismissed), sha, lastPushedAt.Format(time.RFC3339),
	)

	return allowed, dismissed, nil
}

// filteredCommits returns the relevant commits for the evaluation ordered in
// history order, from most to least recent.
func (r *Rule) filteredCommits(ctx context.Context, prctx pull.Context) ([]*pull.Commit, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commits")
	}
	commits = sortCommits(commits, prctx.HeadSHA())

	ignoreUpdates := r.Options.IgnoreUpdateMerges
	ignoreCommits := !r.Options.IgnoreCommitsBy.IsEmpty()

	if !ignoreUpdates && !ignoreCommits {
		return commits, nil
	}

	var filtered []*pull.Commit
	for _, c := range commits {
		if ignoreUpdates {
			if isUpdateMerge(commits, c) {
				continue
			}
		}

		if ignoreCommits {
			ignore, err := isIgnoredCommit(ctx, prctx, &r.Options.IgnoreCommitsBy, c)
			if err != nil {
				return nil, err
			}
			if ignore {
				continue
			}
		}

		filtered = append(filtered, c)
	}
	return filtered, nil
}

func (r *Rule) statusDescription(approved bool, approvers, candidates []*common.Candidate) string {
	if approved {
		if len(approvers) == 0 {
			return "No approval required"
		}

		var names []string
		for _, c := range approvers {
			names = append(names, c.User)
		}
		return fmt.Sprintf("Approved by %s", strings.Join(names, ", "))
	}

	desc := fmt.Sprintf("%d/%d required approvals", len(approvers), r.Requires.Count)
	if disqualified := len(candidates) - len(approvers); disqualified > 0 {
		desc += fmt.Sprintf(". Ignored %s from disqualified users", numberOfApprovals(disqualified))
	}
	return desc
}

func isUpdateMerge(commits []*pull.Commit, c *pull.Commit) bool {
	// must be a simple merge commit (exactly 2 parents)
	if len(c.Parents) != 2 {
		return false
	}

	// must be created via the UI or the API (no local merges)
	if !c.CommittedViaWeb {
		return false
	}

	shas := make(map[string]bool)
	for _, c := range commits {
		shas[c.SHA] = true
	}

	// first parent must exist: it is a commit on the head branch
	// second parent must not exist: it is already in the base branch
	return shas[c.Parents[0]] && !shas[c.Parents[1]]
}

func isIgnoredCommit(ctx context.Context, prctx pull.Context, actors *common.Actors, c *pull.Commit) (bool, error) {
	for _, u := range c.Users() {
		ignored, err := actors.IsActor(ctx, prctx, u)
		if err != nil {
			return false, err
		}
		if !ignored {
			return false, nil
		}
	}
	// either all users are ignored or the commit has no users; only ignore in the first case
	return len(c.Users()) > 0, nil
}

func numberOfApprovals(count int) string {
	if count == 1 {
		return "1 approval"
	}
	return fmt.Sprintf("%d approvals", count)
}

// sortCommits orders commits in history order starting from head. It must be
// called on the unfiltered set of commits.
func sortCommits(commits []*pull.Commit, head string) []*pull.Commit {
	commitsBySHA := make(map[string]*pull.Commit)
	for _, c := range commits {
		commitsBySHA[c.SHA] = c
	}

	ordered := make([]*pull.Commit, 0, len(commits))
	for {
		c, ok := commitsBySHA[head]
		if !ok {
			break
		}
		ordered = append(ordered, c)
		if len(c.Parents) == 0 {
			break
		}
		head = c.Parents[0]
	}
	return ordered
}
