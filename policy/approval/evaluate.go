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
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/rs/zerolog"
)

type evaluator struct {
	root common.Evaluator
}

func (eval *evaluator) Trigger() common.Trigger {
	if eval.root != nil {
		return eval.root.Trigger()
	}
	return common.TriggerStatic
}

func (eval *evaluator) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	if eval.root != nil {
		res = eval.root.Evaluate(ctx, prctx)
	} else {
		zerolog.Ctx(ctx).Debug().Msg("No approval policy defined; skipping")

		res.Status = common.StatusApproved
		res.StatusDescription = "No approval policy defined"
	}

	res.Name = "approval"
	return
}

type RuleRequirement struct {
	rule *Rule
}

func (r *RuleRequirement) Trigger() common.Trigger {
	return r.rule.Trigger()
}

func (r *RuleRequirement) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	log := zerolog.Ctx(ctx).With().Str("rule", r.rule.Name).Logger()
	ctx = log.WithContext(ctx)

	result := r.rule.Evaluate(ctx, prctx)
	if result.Error == nil {
		log.Debug().Msgf("rule evaluation resulted in %s:\"%s\"", result.Status, result.StatusDescription)
	} else {
		// Log rule evaluations that fail at info level so they appear in logs
		// by default, but don't log them as warnings or errors since they
		// don't necessarily break the overall policy (e.g. an 'or' rule can
		// suppress an error if other members are pending or approved.) Having
		// this information in logs is useful to understand the rate of
		// particular types of failures across a policy-bot installation.
		log.Info().Err(result.Error).Msg("rule evaluation resulted in error")
	}

	return result
}

type OrRequirement struct {
	requirements []common.Evaluator
}

func (r *OrRequirement) Trigger() common.Trigger {
	var t common.Trigger
	for _, child := range r.requirements {
		t |= child.Trigger()
	}
	return t
}

func (r *OrRequirement) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	var children []*common.Result
	for _, req := range r.requirements {
		res := req.Evaluate(ctx, prctx)
		children = append(children, &res)
	}

	var err error
	var pending, approved, skipped int
	for _, c := range children {
		if c.Error != nil {
			err = c.Error
			continue
		}

		switch c.Status {
		case common.StatusApproved:
			approved++
		case common.StatusPending:
			pending++
		case common.StatusSkipped:
			skipped++
		}
	}

	var status common.EvaluationStatus
	description := "All of the rules are skipped"

	switch {
	case approved > 0:
		status = common.StatusApproved
		description = "One or more rules approved"
		err = nil
	case pending > 0:
		status = common.StatusPending
		description = "None of the rules are satisfied"
		err = nil
	}

	return common.Result{
		Name:              "or",
		Status:            status,
		StatusDescription: description,
		Error:             err,
		Children:          children,
	}
}

type AndRequirement struct {
	requirements []common.Evaluator
}

func (r *AndRequirement) Trigger() common.Trigger {
	var t common.Trigger
	for _, child := range r.requirements {
		t |= child.Trigger()
	}
	return t
}

func (r *AndRequirement) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	var children []*common.Result
	for _, req := range r.requirements {
		res := req.Evaluate(ctx, prctx)
		children = append(children, &res)
	}

	var err error
	var pending, approved, skipped int
	var pendingDetails []string
	for _, c := range children {
		if c.Error != nil {
			err = c.Error
			continue
		}

		switch c.Status {
		case common.StatusApproved:
			approved++
		case common.StatusPending:
			pending++
			// Capture the pending rule's StatusDescription so the top-level
			// status posted to GitHub names the actual blocker (e.g. the
			// failing has_status checks) instead of just "X/Y rules approved".
			// We collect from every pending rule because in an AND, all of
			// them must clear before merge.
			if d := pendingRuleDetail(c); d != "" {
				pendingDetails = append(pendingDetails, d)
			}
		case common.StatusSkipped:
			skipped++
		}
	}

	var status common.EvaluationStatus
	description := "All of the rules are skipped"

	switch {
	case approved > 0 && pending == 0:
		status = common.StatusApproved
		description = fmt.Sprintf("All rules are approved")
	case pending > 0:
		status = common.StatusPending
		description = fmt.Sprintf("%d/%d rules approved", approved, approved+pending)
		if len(pendingDetails) > 0 {
			description = fmt.Sprintf("%s; %s", description, joinPendingDetails(pendingDetails))
		}
	}

	return common.Result{
		Name:              "and",
		Status:            status,
		StatusDescription: description,
		Error:             err,
		Children:          children,
	}
}

// pendingRuleDetail returns a human-readable, blocker-focused suffix for a
// child rule that is currently pending. The rule's own StatusDescription
// already encodes either the failing condition predicates (set by
// statusDescription in approve.go) or the failing precondition predicate (set
// by Rule.Evaluate when a predicate is not satisfied). We prefer to include
// the rule name so a reader can correlate the message with the policy.yml.
func pendingRuleDetail(c *common.Result) string {
	desc := strings.TrimSpace(c.StatusDescription)
	if desc == "" {
		return ""
	}
	if c.Name == "" {
		return desc
	}
	return fmt.Sprintf("%q: %s", c.Name, desc)
}

// joinPendingDetails concatenates pending-rule details with a stable
// delimiter. We deliberately do not truncate here: the full description is
// rendered on the policy-bot details page and emitted to logs, and GitHub's
// own 140-character commit status limit will truncate the message at post
// time. Truncating internally would hide the actionable detail from the
// details page and logs while only saving bytes that GitHub would discard
// anyway.
func joinPendingDetails(details []string) string {
	return strings.Join(details, " | ")
}
