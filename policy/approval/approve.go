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
	AllowAuthor      bool `yaml:"allow_author"`
	AllowContributor bool `yaml:"allow_contributor"`
	InvalidateOnPush bool `yaml:"invalidate_on_push"`

	IgnoreEditedComments bool          `yaml:"ignore_edited_comments"`
	IgnoreUpdateMerges   bool          `yaml:"ignore_update_merges"`
	IgnoreCommitsBy      common.Actors `yaml:"ignore_commits_by"`

	RequestReview RequestReview `yaml:"request_review"`

	Methods *common.Methods `yaml:"methods"`
}

type RequestReview struct {
	Enabled bool               `yaml:"enabled"`
	Mode    common.RequestMode `yaml:"mode"`
}

func (opts *Options) GetMethods() *common.Methods {
	methods := opts.Methods
	if methods == nil {
		methods = &common.Methods{
			Comments: []string{
				":+1:",
				"👍",
			},
			GithubReview: true,
		}
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
		if m.GithubReview || len(m.GithubReviewCommentPatterns) > 0 {
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

	var predicatesInfo []*common.PredicateInfo

	for _, p := range r.Predicates.Predicates() {
		satisfied, desc, pPredicateInfo, err := p.Evaluate(ctx, prctx)
		predicatesInfo = append(predicatesInfo, pPredicateInfo)
		if err != nil {
			res.Error = errors.Wrap(err, "failed to evaluate predicate")
			return
		}

		if !satisfied {
			log.Debug().Msgf("skipping rule, predicate of type %T was not satisfied", p)

			res.StatusDescription = desc
			if desc == "" {
				res.StatusDescription = "A precondition of this rule was not satisfied"
			}
			res.PredicatesInfo = []*common.PredicateInfo{pPredicateInfo}
			return
		}
	}
	res.PredicatesInfo = predicatesInfo
	approved, msg, err := r.IsApproved(ctx, prctx)
	if err != nil {
		res.Error = errors.Wrap(err, "failed to compute approval status")
		return
	}

	res.StatusDescription = msg
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

	perms := append([]pull.Permission(nil), r.Requires.Permissions...)
	if r.Requires.Admins {
		perms = append(perms, pull.PermissionAdmin)
	}
	if r.Requires.WriteCollaborators {
		perms = append(perms, pull.PermissionWrite)
	}

	return &common.ReviewRequestRule{
		Users:         r.Requires.Users,
		Teams:         r.Requires.Teams,
		Organizations: r.Requires.Organizations,
		Permissions:   perms,
		RequiredCount: r.Requires.Count,
		Mode:          mode,
	}
}

func (r *Rule) IsApproved(ctx context.Context, prctx pull.Context) (bool, string, error) {
	log := zerolog.Ctx(ctx)

	if r.Requires.Count <= 0 {
		log.Debug().Msg("rule requires no approvals")
		return true, "No approval required", nil
	}

	candidates, err := r.filteredCandidates(ctx, prctx)
	if err != nil {
		return false, "", err
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
	if !r.Options.AllowContributor {
		commits, err := r.filteredCommits(ctx, prctx)
		if err != nil {
			return false, "", err
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
	var approvers []string
	for _, c := range candidates {
		if banned[c.User] {
			log.Debug().Str("user", c.User).Msg("rejecting approval by banned user")
			continue
		}

		isApprover, err := r.Requires.IsActor(ctx, prctx, c.User)
		if err != nil {
			return false, "", errors.Wrap(err, "failed to check candidate status")
		}
		if !isApprover {
			log.Debug().Str("user", c.User).Msg("ignoring approval by non-whitelisted user")
			continue
		}

		approvers = append(approvers, c.User)
	}

	log.Debug().Msgf("found %d/%d required approvers", len(approvers), r.Requires.Count)
	remaining := r.Requires.Count - len(approvers)

	if remaining <= 0 {
		msg := fmt.Sprintf("Approved by %s", strings.Join(approvers, ", "))
		return true, msg, nil
	}

	if len(candidates) > 0 && len(approvers) == 0 {
		msg := fmt.Sprintf("%d/%d approvals required. Ignored %s from disqualified users",
			len(approvers),
			r.Requires.Count,
			numberOfApprovals(len(candidates)))
		return false, msg, nil
	}

	msg := fmt.Sprintf("%d/%d approvals required", len(approvers), r.Requires.Count)
	return false, msg, nil
}

func (r *Rule) filteredCandidates(ctx context.Context, prctx pull.Context) ([]*common.Candidate, error) {
	candidates, err := r.Options.GetMethods().Candidates(ctx, prctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get approval candidates")
	}

	sort.Stable(common.CandidatesByCreationTime(candidates))

	if r.Options.IgnoreEditedComments {
		candidates, err = r.filterEditedCandidates(ctx, prctx, candidates)
		if err != nil {
			return nil, err
		}
	}

	if r.Options.InvalidateOnPush {
		candidates, err = r.filterInvalidCandidates(ctx, prctx, candidates)
		if err != nil {
			return nil, err
		}
	}

	return candidates, nil
}

func (r *Rule) filterEditedCandidates(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) ([]*common.Candidate, error) {
	log := zerolog.Ctx(ctx)

	if !r.Options.IgnoreEditedComments {
		return candidates, nil
	}

	var allowedCandidates []*common.Candidate
	for _, candidate := range candidates {
		if r.Options.IgnoreEditedComments {
			if candidate.UpdatedAt == candidate.CreatedAt {
				allowedCandidates = append(allowedCandidates, candidate)
			}
		}
	}

	log.Debug().Msgf("discarded %d candidates with edited comments",
		len(candidates)-len(allowedCandidates))

	return allowedCandidates, nil
}

func (r *Rule) filterInvalidCandidates(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) ([]*common.Candidate, error) {
	log := zerolog.Ctx(ctx)

	if !r.Options.InvalidateOnPush {
		return candidates, nil
	}

	commits, err := r.filteredCommits(ctx, prctx)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return candidates, nil
	}

	last := findLastPushed(commits)
	if last == nil {
		return nil, errors.New("no commit contained a push date")
	}

	var allowedCandidates []*common.Candidate
	for _, candidate := range candidates {
		if candidate.CreatedAt.After(*last.PushedAt) {
			allowedCandidates = append(allowedCandidates, candidate)
		}
	}

	log.Debug().Msgf("discarded %d candidates invalidated by push of %s at %s",
		len(candidates)-len(allowedCandidates),
		last.SHA,
		last.PushedAt.Format(time.RFC3339))

	return allowedCandidates, nil
}

func (r *Rule) filteredCommits(ctx context.Context, prctx pull.Context) ([]*pull.Commit, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commits")
	}

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

func findLastPushed(commits []*pull.Commit) *pull.Commit {
	var last *pull.Commit
	for _, c := range commits {
		if c.PushedAt != nil && (last == nil || c.PushedAt.After(*last.PushedAt)) {
			last = c
		}
	}
	return last
}

func numberOfApprovals(count int) string {
	if count == 1 {
		return "1 approval"
	}
	return fmt.Sprintf("%d approvals", count)
}
