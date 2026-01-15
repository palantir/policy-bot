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
	"sync"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Rule struct {
	Name        string               `yaml:"name,omitempty"`
	Description string               `yaml:"description,omitempty"`
	Predicates  predicate.Predicates `yaml:"if,omitempty"`
	Options     Options              `yaml:"options,omitempty"`
	Requires    Requires             `yaml:"requires,omitempty"`
}

type Requires struct {
	Count      int                  `yaml:"count,omitempty"`
	Actors     common.Actors        `yaml:",inline"`
	Conditions predicate.Predicates `yaml:"conditions,omitempty"`
}

type Defaults struct {
	Options *Options `yaml:"options,omitempty"`
}

func (r *Rule) Trigger() common.Trigger {
	t := common.TriggerCommit

	if r.Requires.Count > 0 {
		m := r.Options.GetMethods()
		if len(m.GetComments()) > 0 || len(m.GetCommentPatterns()) > 0 {
			t |= common.TriggerComment
		}
		if len(m.GetBodyPatterns()) > 0 {
			t |= common.TriggerPullRequest
		}
		if m.IsGithubReview() || len(m.GetGithubReviewCommentPatterns()) > 0 {
			t |= common.TriggerReview
		}
	}

	for _, c := range r.Requires.Conditions.Predicates() {
		t |= c.Trigger()
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

	approved, result, err := r.IsApproved(ctx, prctx, candidates)
	if err != nil {
		res.Error = errors.Wrap(err, "failed to compute approval status")
		return
	}

	res.Requires = result
	res.Dismissals = dismissals
	res.StatusDescription = statusDescription(approved, result, candidates)

	if approved {
		res.Status = common.StatusApproved
	} else {
		res.Status = common.StatusPending
		res.ReviewRequestRule = r.getReviewRequestRule()
	}

	return
}

func (r *Rule) getReviewRequestRule() *common.ReviewRequestRule {
	rr := r.Options.GetRequestReview()
	if !rr.Enabled {
		return nil
	}

	mode := rr.Mode
	if mode == "" {
		mode = common.RequestModeRandomUsers
	}

	requestedCount := rr.Count
	if requestedCount == 0 {
		requestedCount = r.Requires.Count
	}

	return &common.ReviewRequestRule{
		Users:          r.Requires.Actors.Users,
		Teams:          r.Requires.Actors.Teams,
		Organizations:  r.Requires.Actors.Organizations,
		Permissions:    r.Requires.Actors.GetPermissions(),
		Codeowners:     r.Requires.Actors.Codeowners,
		RequiredCount:  r.Requires.Count,
		RequestedCount: requestedCount,
		Mode:           mode,
	}
}

func (r *Rule) IsApproved(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) (bool, common.RequiresResult, error) {
	approvedByActors, approvers, ownershipGroups, err := r.isApprovedByActors(ctx, prctx, candidates)
	if err != nil {
		return false, common.RequiresResult{}, err
	}

	approvedByConditions, conditions, err := r.isApprovedByConditions(ctx, prctx)
	if err != nil {
		return false, common.RequiresResult{}, err
	}

	result := common.RequiresResult{
		Count:           r.Requires.Count,
		Actors:          r.Requires.Actors,
		Approvers:       approvers,
		Conditions:      conditions,
		OwnershipGroups: ownershipGroups,
	}
	return approvedByActors && approvedByConditions, result, nil
}

func (r *Rule) isApprovedByActors(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) (bool, []*common.Candidate, []common.OwnershipGroupResult, error) {
	log := zerolog.Ctx(ctx)

	if r.Requires.Count <= 0 && !r.Requires.Actors.Codeowners {
		log.Debug().Msg("rule requires no approvals")
		return true, nil, nil, nil
	}

	log.Debug().Msgf("found %d candidates for approval", len(candidates))

	// collect users "banned" by approval options
	banned := make(map[string]bool)

	// "author" is the user who opened the PR
	// if contributors are allowed, the author counts as a contributor
	author := prctx.Author()

	if !r.Options.IsAllowAuthor() && !r.Options.IsAllowContributor() {
		banned[author] = true
	}

	// "contributor" is any user who added a commit to the PR
	if !r.Options.IsAllowContributor() && !r.Options.IsAllowNonAuthorContributor() {
		commits, err := r.filteredCommits(ctx, prctx)
		if err != nil {
			return false, nil, nil, err
		}

		for _, c := range commits {
			for _, u := range c.Users() {
				if u != author {
					banned[u] = true
				}
			}
		}
	}

	// If codeowners is enabled, delegate to codeowner group logic
	if r.Requires.Actors.Codeowners {
		return r.isApprovedByCodeownerGroups(ctx, prctx, candidates, banned)
	}

	// filter real approvers using banned status and required membership
	var approvers []*common.Candidate
	for _, c := range candidates {
		if banned[c.User()] {
			log.Debug().Str("user", c.User()).Msg("rejecting approval by banned user")
			continue
		}

		isApprover, err := r.Requires.Actors.IsActor(ctx, prctx, c.User())
		if err != nil {
			return false, nil, nil, errors.Wrap(err, "failed to check candidate status")
		}
		if !isApprover {
			log.Debug().Str("user", c.User()).Msg("ignoring approval by non-required user")
			continue
		}

		approvers = append(approvers, c)
	}

	log.Debug().Msgf("found %d/%d required approvers", len(approvers), r.Requires.Count)
	return len(approvers) >= r.Requires.Count, approvers, nil, nil
}

func (r *Rule) isApprovedByConditions(ctx context.Context, prctx pull.Context) (bool, []*common.PredicateResult, error) {
	log := zerolog.Ctx(ctx)

	conditions := r.Requires.Conditions.Predicates()
	if len(conditions) == 0 {
		log.Debug().Msg("rule requires no conditions")
		return true, nil, nil
	}

	var results []*common.PredicateResult
	var approved int

	for _, c := range conditions {
		result, err := c.Evaluate(ctx, prctx)
		if err != nil {
			return false, nil, errors.Wrap(err, "failed to evaluate condition")
		}
		if result.Satisfied {
			approved++
		}
		results = append(results, result)
	}

	log.Debug().Msgf("found %d/%d required conditions", approved, len(conditions))
	return approved == len(conditions), results, nil
}

// FilteredCandidates returns the potential approval candidates and any
// candidates that should be dimissed due to rule options.
func (r *Rule) FilteredCandidates(ctx context.Context, prctx pull.Context) ([]*common.Candidate, []*common.Dismissal, error) {
	if r.Requires.Count <= 0 && !r.Requires.Actors.Codeowners {
		return nil, nil, nil
	}

	candidates, err := r.Options.GetMethods().Candidates(ctx, prctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get approval candidates")
	}

	sort.Stable(common.CandidatesByCreationTime(candidates))

	var editDismissals []*common.Dismissal
	if r.Options.IsIgnoreEditedComments() {
		candidates, editDismissals, err = r.filterEditedCandidates(ctx, prctx, candidates)
		if err != nil {
			return nil, nil, err
		}
	}

	var pushDismissals []*common.Dismissal
	if r.Options.IsInvalidateOnPush() {
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

func (r *Rule) filterEditedCandidates(ctx context.Context, _ pull.Context, candidates []*common.Candidate) ([]*common.Candidate, []*common.Dismissal, error) {
	log := zerolog.Ctx(ctx)

	if !r.Options.IsIgnoreEditedComments() {
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

	ignoreUpdates := r.Options.IsIgnoreUpdateMerges()
	ignoreCommitsBy := r.Options.GetIgnoreCommitsBy()
	ignoreCommits := !ignoreCommitsBy.IsZero()

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
			ignore, err := isIgnoredCommit(ctx, prctx, &ignoreCommitsBy, c)
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

func statusDescription(approved bool, result common.RequiresResult, candidates []*common.Candidate) string {
	hasActors := result.Count > 0
	hasConditions := len(result.Conditions) > 0
	hasOwnershipGroups := len(result.OwnershipGroups) > 0

	if approved {
		if !hasActors && !hasConditions && !hasOwnershipGroups {
			return "No approval required"
		}

		var desc strings.Builder
		if hasOwnershipGroups {
			desc.WriteString(ownershipGroupsStatusDescription(result.OwnershipGroups))
		} else {
			desc.WriteString("Approved by ")
			for i, c := range result.Approvers {
				if i > 0 {
					desc.WriteString(", ")
				}
				desc.WriteString(c.User())
			}
		}
		if hasConditions {
			if hasActors || hasOwnershipGroups {
				desc.WriteString(" and ")
			}
			desc.WriteString("required conditions")
		}

		return desc.String()
	}

	var desc strings.Builder
	if hasOwnershipGroups {
		desc.WriteString(ownershipGroupsStatusDescription(result.OwnershipGroups))
	} else if hasActors {
		fmt.Fprintf(&desc, "%d/%d required approvals", len(result.Approvers), result.Count)
	}
	if hasConditions {
		if hasActors || hasOwnershipGroups {
			desc.WriteString(" and ")
		}

		successful := 0
		for _, c := range result.Conditions {
			if c.Satisfied {
				successful++
			}
		}
		fmt.Fprintf(&desc, "%d/%d required conditions", successful, len(result.Conditions))
	}
	if disqualified := len(candidates) - len(result.Approvers); hasActors && !hasOwnershipGroups && disqualified > 0 {
		fmt.Fprintf(&desc, ". Ignored %s from disqualified users", numberOfApprovals(disqualified))
	}
	return desc.String()
}

func ownershipGroupsStatusDescription(groups []common.OwnershipGroupResult) string {
	var satisfied int
	approverSet := make(map[string]struct{})

	for _, g := range groups {
		if g.Satisfied {
			satisfied++
			for _, a := range g.Approvers {
				approverSet[a] = struct{}{}
			}
		}
	}

	if satisfied < len(groups) {
		return fmt.Sprintf("%d/%d ownership groups covered", satisfied, len(groups))
	}

	approvers := make([]string, 0, len(approverSet))
	for a := range approverSet {
		approvers = append(approvers, a)
	}
	sort.Strings(approvers)
	return fmt.Sprintf("Approved by %s covering %d ownership groups", strings.Join(approvers, ", "), len(groups))
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

// isApprovedByCodeownerGroups checks if all ownership groups have at least one
// approved candidate. Returns true if all groups are satisfied, the list of
// approvers, and the results for each ownership group.
func (r *Rule) isApprovedByCodeownerGroups(
	ctx context.Context,
	prctx pull.Context,
	candidates []*common.Candidate,
	banned map[string]bool,
) (bool, []*common.Candidate, []common.OwnershipGroupResult, error) {
	log := zerolog.Ctx(ctx)

	co, err := prctx.Codeowners()
	if err != nil {
		return false, nil, nil, errors.Wrap(err, "failed to get codeowners")
	}
	if co == nil {
		// No CODEOWNERS file - no codeowner requirements
		log.Debug().Msg("no CODEOWNERS file found, skipping codeowner group check")
		return true, nil, nil, nil
	}

	groups := co.OwnershipGroups()
	if len(groups) == 0 {
		log.Debug().Msg("no ownership groups found, skipping codeowner group check")
		return true, nil, nil, nil
	}

	log.Debug().Msgf("found %d ownership groups to satisfy", len(groups))

	// Pre-filter candidates to exclude banned users
	validCandidates := make([]*common.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if !banned[c.User()] {
			validCandidates = append(validCandidates, c)
		}
	}

	// Collect all unique (team, user) pairs that need membership checks
	type memberCheck struct {
		team string
		user string
	}
	checksNeeded := make(map[memberCheck]struct{})
	for _, group := range groups {
		for _, owner := range group.Owners {
			ownerType, name := pull.ParseCodeowner(owner)
			if ownerType == "team" {
				for _, c := range validCandidates {
					checksNeeded[memberCheck{team: name, user: c.User()}] = struct{}{}
				}
			}
		}
	}

	// Run all team membership checks in parallel
	membershipResults := make(map[memberCheck]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(checksNeeded))

	for check := range checksNeeded {
		wg.Go(func() {
			isMember, err := prctx.IsTeamMember(check.team, check.user)
			if err != nil {
				errChan <- errors.Wrapf(err, "failed to check team membership for %s in %s", check.user, check.team)
				return
			}
			mu.Lock()
			membershipResults[check] = isMember
			mu.Unlock()
		})
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	if err := <-errChan; err != nil {
		return false, nil, nil, err
	}

	// Process results using the cached membership data
	results := make([]common.OwnershipGroupResult, len(groups))
	var approvers []*common.Candidate
	approverSet := make(map[string]struct{})

	for i, group := range groups {
		results[i] = common.OwnershipGroupResult{
			Key:    group.Key,
			Owners: group.Owners,
			Files:  group.Files,
		}

		for _, c := range validCandidates {
			isMember := false
			for _, owner := range group.Owners {
				ownerType, name := pull.ParseCodeowner(owner)
				switch ownerType {
				case "user":
					if strings.EqualFold(c.User(), name) {
						isMember = true
					}
				case "team":
					if membershipResults[memberCheck{team: name, user: c.User()}] {
						isMember = true
					}
				}
				if isMember {
					break
				}
			}

			if isMember {
				results[i].Satisfied = true
				results[i].Approvers = append(results[i].Approvers, c.User())
				if _, exists := approverSet[c.User()]; !exists {
					approvers = append(approvers, c)
					approverSet[c.User()] = struct{}{}
				}
			}
		}
	}

	// Count satisfied groups
	satisfiedCount := 0
	for _, result := range results {
		if result.Satisfied {
			satisfiedCount++
		}
	}

	log.Debug().Msgf("%d/%d ownership groups satisfied", satisfiedCount, len(results))

	return satisfiedCount == len(results), approvers, results, nil
}

