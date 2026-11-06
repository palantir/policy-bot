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
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
)

const (
	rulesetLookupRetryDelay = 500 * time.Millisecond

	MetricsKeyRulesetLookupErrors  = "rulesets.lookup.errors"
	MetricsKeyRulesetLookupLatency = "rulesets.lookup.latency"
	MetricsKeyRulesetChecksPosted  = "rulesets.checks.posted"
	MetricsKeyRulesetChecksIgnored = "rulesets.checks.ignored"

	rulesetContextInfix = "ruleset:"
)

// reasons for ignoring a required check that uses the policy-bot prefix
const (
	ignoreReasonUnpinned         = "unpinned"
	ignoreReasonWrongIntegration = "wrong_integration"
	ignoreReasonUnknownRuleset   = "unknown_ruleset"
	ignoreReasonNameMismatch     = "name_mismatch"
)

// RulesetChecks finds the ruleset status checks that policy-bot should post
// for pull requests targeting a branch.
type RulesetChecks interface {
	// RequiredChecks returns the de-duplicated, sorted contexts of the
	// ruleset checks to post for the branch. If an error is returned, callers
	// must not post any ruleset checks.
	RequiredChecks(ctx context.Context, client *github.Client, owner, repo, branch string) ([]string, error)
}

// RulesetContext returns the status context for the named ruleset.
func RulesetContext(statusCheckContext, rulesetName string) string {
	return statusCheckContext + ": " + rulesetContextInfix + rulesetName
}

// RulesetCheckFinder implements RulesetChecks using the GitHub rules API.
// It relies on the client's HTTP cache, which GitHub allows to reuse rules
// and rulesets responses for up to 60 seconds.
//
// A required check is only returned if it uses the policy-bot ruleset prefix,
// is pinned to policy-bot's GitHub App, and names the ruleset that requires
// it. This ensures that policy-bot never posts a shared check for a branch
// unless that branch is protected by the ruleset that requires the check.
type RulesetCheckFinder struct {
	AppID              int64
	StatusCheckContext string
	Registry           metrics.Registry

	retryDelay time.Duration
}

func NewRulesetCheckFinder(appID int64, statusCheckContext string, registry metrics.Registry) *RulesetCheckFinder {
	return &RulesetCheckFinder{
		AppID:              appID,
		StatusCheckContext: statusCheckContext,
		Registry:           registry,
		retryDelay:         rulesetLookupRetryDelay,
	}
}

func (f *RulesetCheckFinder) RequiredChecks(ctx context.Context, client *github.Client, owner, repo, branch string) ([]string, error) {
	rules, err := f.getRules(ctx, client, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	// avoid looking up names when no rule could produce a check
	prefix := RulesetContext(f.StatusCheckContext, "")
	if !slices.ContainsFunc(rules, func(r *github.RequiredStatusChecksBranchRule) bool {
		return slices.ContainsFunc(r.Parameters.RequiredStatusChecks, func(c *github.RuleStatusCheck) bool {
			return strings.HasPrefix(c.Context, prefix)
		})
	}) {
		return nil, nil
	}

	names, err := f.getRulesetNames(ctx, client, owner, repo, false)
	if err != nil {
		return nil, err
	}

	// the cached names may be stale if a ruleset was created after the rules
	if slices.ContainsFunc(rules, func(r *github.RequiredStatusChecksBranchRule) bool {
		_, ok := names[r.RulesetID]
		return !ok
	}) {
		if names, err = f.getRulesetNames(ctx, client, owner, repo, true); err != nil {
			return nil, err
		}
	}

	return f.selectChecks(ctx, rules, names), nil
}

// selectChecks returns the contexts of the required checks in rules that
// policy-bot should post, logging and counting any ignored checks.
func (f *RulesetCheckFinder) selectChecks(ctx context.Context, rules []*github.RequiredStatusChecksBranchRule, names map[int64]string) []string {
	logger := zerolog.Ctx(ctx)
	prefix := RulesetContext(f.StatusCheckContext, "")

	var checks []string
	for _, rule := range rules {
		rulesetName, hasName := names[rule.RulesetID]

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
			}

			if reason != "" {
				logger.Info().
					Str("context", check.Context).
					Int64("ruleset_id", rule.RulesetID).
					Str("ruleset_name", rulesetName).
					Str("ruleset_source", rule.RulesetSource).
					Str("reason", reason).
					Msg("Ignoring required ruleset status check")
				metrics.GetOrRegisterCounter(fmt.Sprintf("%s[reason:%s]", MetricsKeyRulesetChecksIgnored, reason), f.Registry).Inc(1)
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
	err := f.timeLookup(func() error {
		opts := &github.ListOptions{PerPage: 100}
		for {
			var page *github.BranchRules
			var res *github.Response
			err := f.retry(ctx, func() (err error) {
				// the client escapes the branch name in the path
				page, res, err = client.Repositories.ListRulesForBranch(ctx, owner, repo, branch, opts)
				return err
			})
			if err != nil {
				return errors.Wrapf(err, "failed to list rules for branch %q", branch)
			}
			if page != nil {
				rules = append(rules, page.RequiredStatusChecks...)
			}
			if res.NextPage == 0 {
				return nil
			}
			opts.Page = res.NextPage
		}
	})
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// getRulesetNames returns a map from ruleset ID to name for all rulesets
// that apply to the repository. If revalidate is true, cached responses are
// revalidated with GitHub before use.
func (f *RulesetCheckFinder) getRulesetNames(ctx context.Context, client *github.Client, owner, repo string, revalidate bool) (map[int64]string, error) {
	names := make(map[int64]string)
	err := f.timeLookup(func() error {
		page := 1
		for {
			var rulesets []*github.RepositoryRuleset
			var res *github.Response
			err := f.retry(ctx, func() error {
				// build the request directly because GetAllRulesets does not
				// allow setting headers; max-age=0 makes the HTTP cache send
				// a conditional request instead of using a cached response
				u := fmt.Sprintf("repos/%v/%v/rulesets?includes_parents=true&per_page=100&page=%d", owner, repo, page)
				req, err := client.NewRequest(ctx, http.MethodGet, u, nil)
				if err != nil {
					return err
				}
				if revalidate {
					req.Header.Set("Cache-Control", "max-age=0")
				}
				rulesets = nil
				res, err = client.Do(req, &rulesets)
				return err
			})
			if err != nil {
				return errors.Wrap(err, "failed to list rulesets")
			}
			for _, rs := range rulesets {
				names[rs.GetID()] = rs.Name
			}
			if res.NextPage == 0 {
				return nil
			}
			page = res.NextPage
		}
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// retry calls fn once more after a short delay if it returns a server error.
// Timeouts are not retried because they already waited for the full client
// timeout, and pull requests get their checks on their next event anyway.
func (f *RulesetCheckFinder) retry(ctx context.Context, fn func() error) error {
	err := fn()
	if err == nil || !isServerError(err) {
		return err
	}

	zerolog.Ctx(ctx).Debug().Err(err).Msgf("Retrying ruleset lookup in %s", f.retryDelay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(f.retryDelay):
		return fn()
	}
}

func (f *RulesetCheckFinder) timeLookup(fn func() error) error {
	start := time.Now()
	err := fn()
	metrics.GetOrRegisterTimer(MetricsKeyRulesetLookupLatency, f.Registry).UpdateSince(start)
	if err != nil {
		metrics.GetOrRegisterCounter(MetricsKeyRulesetLookupErrors, f.Registry).Inc(1)
	}
	return err
}
