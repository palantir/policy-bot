// Copyright 2026 Palantir Technologies, Inc.
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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// reasons for ignoring a required check that uses the policy-bot prefix
const (
	ignoreReasonUnpinned         = "unpinned"
	ignoreReasonWrongIntegration = "wrong_integration"
	ignoreReasonUnknownRuleset   = "unknown_ruleset"
	ignoreReasonNameMismatch     = "name_mismatch"
	ignoreReasonShadowed         = "shadowed"
)

// rulesetInfo is the metadata of a ruleset that applies to a repository.
type rulesetInfo struct {
	name  string
	scope int
}

// rulesetScope ranks ruleset sources so that rulesets owned by broader scopes
// take precedence. Unknown sources rank lowest.
func rulesetScope(t github.RulesetSourceType) int {
	switch t {
	case github.RulesetSourceTypeEnterprise:
		return 2
	case github.RulesetSourceTypeOrganization:
		return 1
	default:
		return 0
	}
}

// RulesetChecks finds the ruleset status checks that policy-bot should post
// for pull requests targeting a branch.
type RulesetChecks interface {
	// RequiredChecks returns the de-duplicated, sorted contexts of the
	// ruleset checks to post for the branch. If an error is returned, callers
	// must not post a success to any ruleset check.
	RequiredChecks(ctx context.Context, client *github.Client, owner, repo, branch string) ([]string, error)
}

// RulesetContext returns the status context for the named ruleset.
func RulesetContext(statusCheckContext, rulesetName string) string {
	return fmt.Sprintf("%s: ruleset:%s", statusCheckContext, rulesetName)
}

// RulesetCheckFinder implements RulesetChecks using the GitHub rules API.
//
// A required check is only returned if it uses the policy-bot ruleset prefix,
// is pinned to policy-bot's GitHub App, and names the ruleset that requires
// it. This ensures that policy-bot never posts a shared check for a branch
// unless that branch is protected by the ruleset that requires the check.
type RulesetCheckFinder struct {
	AppID              int64
	StatusCheckContext string

	retryDelay time.Duration
}

func NewRulesetCheckFinder(appID int64, statusCheckContext string) *RulesetCheckFinder {
	return &RulesetCheckFinder{
		AppID:              appID,
		StatusCheckContext: statusCheckContext,
		retryDelay:         defaultRetryDelay,
	}
}

func (f *RulesetCheckFinder) RequiredChecks(ctx context.Context, client *github.Client, owner, repo, branch string) ([]string, error) {
	rules, err := f.getRules(ctx, client, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	// avoid looking up rulesets when no rule requires a policy-bot ruleset check
	if !f.rulesRequirePolicyBot(rules) {
		return nil, nil
	}

	rulesets, err := f.getRulesets(ctx, client, owner, repo)
	if err != nil {
		return nil, err
	}

	return f.selectChecks(ctx, rules, rulesets), nil
}

// rulesRequirePolicyBot returns true if any rule requires a check with the
// policy-bot ruleset prefix.
func (f *RulesetCheckFinder) rulesRequirePolicyBot(rules []*github.RequiredStatusChecksBranchRule) bool {
	prefix := RulesetContext(f.StatusCheckContext, "")
	for _, rule := range rules {
		for _, check := range rule.Parameters.RequiredStatusChecks {
			if strings.HasPrefix(check.Context, prefix) {
				return true
			}
		}
	}
	return false
}

// selectChecks returns the contexts of the required checks in rules that
// policy-bot should post, logging any ignored checks.
func (f *RulesetCheckFinder) selectChecks(ctx context.Context, rules []*github.RequiredStatusChecksBranchRule, rulesets map[int64]rulesetInfo) []string {
	logger := zerolog.Ctx(ctx)
	prefix := RulesetContext(f.StatusCheckContext, "")

	// a name used by a broader-scope ruleset belongs to that ruleset, so a
	// repository admin can't reuse an organization ruleset's name to post its
	// check for branches the organization ruleset does not protect
	topScope := make(map[string]int)
	for _, rs := range rulesets {
		topScope[rs.name] = max(topScope[rs.name], rs.scope)
	}

	var checks []string
	for _, rule := range rules {
		ruleset, hasName := rulesets[rule.RulesetID]
		rulesetName := ruleset.name

		for _, check := range rule.Parameters.RequiredStatusChecks {
			requiredName, ok := strings.CutPrefix(check.Context, prefix)
			if !ok {
				continue
			}

			var reason string
			switch {
			case check.IntegrationID == nil:
				reason = ignoreReasonUnpinned
			case *check.IntegrationID != f.AppID:
				reason = ignoreReasonWrongIntegration
			case !hasName:
				reason = ignoreReasonUnknownRuleset
			case requiredName != rulesetName:
				reason = ignoreReasonNameMismatch
			case ruleset.scope < topScope[rulesetName]:
				reason = ignoreReasonShadowed
			}

			if reason != "" {
				logger.Info().
					Str("context", check.Context).
					Int64("ruleset_id", rule.RulesetID).
					Str("ruleset_name", rulesetName).
					Str("ruleset_source", rule.RulesetSource).
					Str("reason", reason).
					Msg("Ignoring required ruleset status check")
				continue
			}

			if !slices.Contains(checks, check.Context) {
				checks = append(checks, check.Context)
			}
		}
	}

	slices.Sort(checks)
	return checks
}

func (f *RulesetCheckFinder) getRules(ctx context.Context, client *github.Client, owner, repo, branch string) ([]*github.RequiredStatusChecksBranchRule, error) {
	var rules []*github.RequiredStatusChecksBranchRule
	opts := &github.ListOptions{PerPage: 100}
	for {
		var page *github.BranchRules
		var res *github.Response
		err := withRetry(ctx, f.retryDelay, func() (err error) {
			// the client escapes the branch name in the path
			page, res, err = client.Repositories.ListRulesForBranch(ctx, owner, repo, branch, opts)
			return err
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list rules for branch %q", branch)
		}
		if page != nil {
			rules = append(rules, page.RequiredStatusChecks...)
		}
		if res.NextPage == 0 {
			return rules, nil
		}
		opts.Page = res.NextPage
	}
}

// getRulesets returns the rulesets that apply to the repository, including
// organization and enterprise rulesets, by ID.
func (f *RulesetCheckFinder) getRulesets(ctx context.Context, client *github.Client, owner, repo string) (map[int64]rulesetInfo, error) {
	infos := make(map[int64]rulesetInfo)
	opts := &github.RepositoryListRulesetsOptions{
		IncludesParents: new(true),
		ListOptions:     github.ListOptions{PerPage: 100},
	}
	for {
		var rulesets []*github.RepositoryRuleset
		var res *github.Response
		err := withRetry(ctx, f.retryDelay, func() (err error) {
			rulesets, res, err = client.Repositories.GetAllRulesets(ctx, owner, repo, opts)
			return err
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list rulesets")
		}
		for _, rs := range rulesets {
			var source github.RulesetSourceType
			if rs.SourceType != nil {
				source = *rs.SourceType
			}
			infos[rs.GetID()] = rulesetInfo{name: rs.Name, scope: rulesetScope(source)}
		}
		if res.NextPage == 0 {
			return infos, nil
		}
		opts.Page = res.NextPage
	}
}
