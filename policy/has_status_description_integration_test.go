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

package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestHasStatusDescriptiveErrors_Integration drives the full policy
// evaluation pipeline using a real .policy.yml that mirrors how FINN
// repositories pin a list of CI status checks as approval conditions.
// The test asserts that the StatusDescription produced by the top-level
// evaluator (the same value posted to GitHub as the policy-bot status
// check description and rendered on the policy-bot details page) names
// the specific checks that are blocking the merge.
//
// This is an integration test in the sense that it exercises the same
// ParsePolicy -> evaluator.Evaluate -> AndRequirement.Evaluate ->
// Rule.Evaluate -> HasStatus.Evaluate chain that the production webhook
// handler invokes. We deliberately do not spin up a docker-compose
// environment here because policy-bot has no docker-compose-driven
// integration harness today and the descriptive-errors change lives
// entirely inside the in-process evaluator. See README.md "Development"
// section for the supported test entry point (./godelw verify).
func TestHasStatusDescriptiveErrors_Integration(t *testing.T) {
	ctx := context.Background()

	// A representative FINN-shaped .policy.yml: a single rule that
	// requires four status checks to succeed. Names include realistic
	// characters (brackets, spaces) that we have seen break ad-hoc
	// regex-based check parsers; the descriptive errors must survive
	// those characters unmodified.
	const policyYAML = `
approval_rules:
  - name: required ci checks
    requires:
      conditions:
        has_status:
          statuses:
            - "pre-commit-check"
            - "[Console_backend] SonarCloud Code Analysis"
            - "Tests with Coverage"
            - "Validate GrowthBook Link"
policy:
  approval:
    - "required ci checks"
`

	parseEvaluator := func(t *testing.T) common.Evaluator {
		t.Helper()
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte(policyYAML), &cfg))
		eval, err := ParsePolicy(&cfg, nil)
		require.NoError(t, err)
		return eval
	}

	t.Run("all_required_status_checks_succeed", func(t *testing.T) {
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"pre-commit-check":                           "success",
				"[Console_backend] SonarCloud Code Analysis": "success",
				"Tests with Coverage":                        "success",
				"Validate GrowthBook Link":                   "success",
			},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusApproved, res.Status, "policy should be approved when all checks succeed")
	})

	t.Run("missing_check_is_named_in_status_description", func(t *testing.T) {
		// Mirrors the exact failure mode we hit on DEV-2637: sync PRs
		// blocked by "policy-bot: main = ERROR" with no actionable
		// detail. The missing pre-commit-check check must be named in
		// the status description (which is what GitHub renders).
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"[Console_backend] SonarCloud Code Analysis": "success",
				"Tests with Coverage":                        "success",
				"Validate GrowthBook Link":                   "success",
			},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "Waiting on: pre-commit-check",
			"status description must name the missing check; got %q", res.StatusDescription)
	})

	t.Run("failing_check_is_named_with_failure_label", func(t *testing.T) {
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"pre-commit-check":                           "success",
				"[Console_backend] SonarCloud Code Analysis": "success",
				"Tests with Coverage":                        "failure",
				"Validate GrowthBook Link":                   "success",
			},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "Failing: Tests with Coverage",
			"status description must name the failing check; got %q", res.StatusDescription)
	})

	t.Run("skipped_check_is_distinguished_from_failure", func(t *testing.T) {
		// When a check completes with a non-success conclusion that is
		// not in the allowed conclusions, the description must
		// distinguish the conclusion (skipped, cancelled, etc.) from a
		// plain failure so a reader can decide whether to retry, rerun,
		// or fix a flaky check.
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"pre-commit-check":                           "success",
				"[Console_backend] SonarCloud Code Analysis": "success",
				"Tests with Coverage":                        "success",
				"Validate GrowthBook Link":                   "skipped",
			},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "Validate GrowthBook Link",
			"status description must name the skipped check; got %q", res.StatusDescription)
		assert.True(t,
			strings.Contains(res.StatusDescription, "Skipped") || strings.Contains(res.StatusDescription, "skipped"),
			"status description must mark the check as skipped, not failed; got %q", res.StatusDescription)
		assert.NotContains(t, res.StatusDescription, "Failing:",
			"a skipped check must not be reported as a failure; got %q", res.StatusDescription)
	})

	t.Run("multiple_blocker_categories_appear_together", func(t *testing.T) {
		// One missing, one failing, one skipped. All three must surface
		// in the same status description so the reader sees the
		// complete blocker set without having to refresh the PR.
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"[Console_backend] SonarCloud Code Analysis": "success",
				"Tests with Coverage":                        "failure",
				"Validate GrowthBook Link":                   "skipped",
			},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "Waiting on: pre-commit-check",
			"missing check must surface; got %q", res.StatusDescription)
		assert.Contains(t, res.StatusDescription, "Failing: Tests with Coverage",
			"failing check must surface; got %q", res.StatusDescription)
		assert.Contains(t, res.StatusDescription, "Validate GrowthBook Link",
			"skipped check must surface; got %q", res.StatusDescription)
	})

	t.Run("rule_name_appears_in_aggregated_description", func(t *testing.T) {
		// When multiple top-level rules combine via the implicit AND,
		// each pending rule must contribute its name so a reader can
		// correlate the blocker with the policy.yml entry.
		const multiRulePolicy = `
approval_rules:
  - name: backend-ci
    requires:
      conditions:
        has_status:
          statuses: ["backend-tests"]
  - name: frontend-ci
    requires:
      conditions:
        has_status:
          statuses: ["frontend-tests"]
policy:
  approval:
    - "backend-ci"
    - "frontend-ci"
`
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte(multiRulePolicy), &cfg))
		eval, err := ParsePolicy(&cfg, nil)
		require.NoError(t, err)

		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"backend-tests": "success",
				// frontend-tests is missing
			},
		}
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "frontend-ci",
			"status description must name the blocking rule; got %q", res.StatusDescription)
		assert.Contains(t, res.StatusDescription, "frontend-tests",
			"status description must name the missing check; got %q", res.StatusDescription)
	})

	t.Run("description_includes_rule_count_for_compatibility", func(t *testing.T) {
		// We must preserve the historical "X/Y rules approved" prefix
		// so external dashboards or alerting that grep this string
		// continue to work. The new detail is appended after the
		// prefix, not in place of it.
		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{},
		}
		eval := parseEvaluator(t)
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusPending, res.Status)
		assert.Contains(t, res.StatusDescription, "0/1 rules approved",
			"legacy prefix must be preserved; got %q", res.StatusDescription)
	})

	t.Run("skipped_conclusion_in_allowed_list_is_treated_as_success", func(t *testing.T) {
		// When the policy explicitly allows "skipped" as a successful
		// conclusion, a skipped check must not appear in the
		// description and the rule must be approved.
		const allowedSkippedPolicy = `
approval_rules:
  - name: tolerant-ci
    requires:
      conditions:
        has_status:
          conclusions: ["success", "skipped"]
          statuses: ["optional-lint"]
policy:
  approval:
    - "tolerant-ci"
`
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte(allowedSkippedPolicy), &cfg))
		eval, err := ParsePolicy(&cfg, nil)
		require.NoError(t, err)

		prctx := &pulltest.Context{
			LatestStatusesValue: map[string]string{
				"optional-lint": "skipped",
			},
		}
		res := eval.Evaluate(ctx, prctx)
		require.NoError(t, res.Error)
		assert.Equal(t, common.StatusApproved, res.Status,
			"skipped should approve when in allowed conclusions; got status %s", res.Status)
	})
}
