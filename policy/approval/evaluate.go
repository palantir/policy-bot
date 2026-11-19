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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/rs/zerolog"
)

type Evaluator interface {
	common.Evaluator

	// Rules returns all rules in the tree without evaluating their conditions.
	Rules() []*Rule

	// FindRule searches the named slot. An empty slot name searches outside slots.
	FindRule(slot, name string) *Rule
}

type evaluator struct {
	root Evaluator
}

func (eval *evaluator) FindRule(slot, name string) *Rule {
	if eval.root != nil {
		return eval.root.FindRule(slot, name)
	}
	return nil
}

func (eval *evaluator) Rules() []*Rule {
	if eval.root != nil {
		return eval.root.Rules()
	}
	return nil
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

type SlotRule struct {
	Name     string
	Required bool
	Child    Evaluator
}

func (r *SlotRule) FindRule(slot, name string) *Rule {
	if slot != r.Name || r.Child == nil {
		return nil
	}
	return r.Child.FindRule("", name)
}

func (r *SlotRule) Rules() []*Rule {
	if r.Child != nil {
		return r.Child.Rules()
	}
	return nil
}

func (r *SlotRule) Trigger() common.Trigger {
	if r.Child != nil {
		return r.Child.Trigger()
	}
	return common.TriggerStatic
}

func (r *SlotRule) Evaluate(ctx context.Context, prctx pull.Context) common.Result {
	if r.Child == nil {
		return common.Result{
			Name:              r.Name,
			Status:            common.StatusSkipped,
			StatusDescription: "Slot is empty",
			Slot:              &common.SlotResult{Name: r.Name, Empty: true},
		}
	}
	result := r.Child.Evaluate(ctx, prctx)
	result.Slot = &common.SlotResult{Name: r.Name}
	return result
}

type RuleRequirement struct {
	rule *Rule
}

func (r *RuleRequirement) FindRule(slot, name string) *Rule {
	if slot == "" && r.rule.Name == name {
		return r.rule
	}
	return nil
}

func (r *RuleRequirement) Rules() []*Rule {
	return []*Rule{r.rule}
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
	requirements []Evaluator
}

func (r *OrRequirement) FindRule(slot, name string) *Rule {
	for _, child := range r.requirements {
		if rule := child.FindRule(slot, name); rule != nil {
			return rule
		}
	}
	return nil
}

func (r *OrRequirement) Rules() []*Rule {
	var rules []*Rule
	for _, child := range r.requirements {
		rules = append(rules, child.Rules()...)
	}
	return rules
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
	requirements []Evaluator
}

func (r *AndRequirement) FindRule(slot, name string) *Rule {
	for _, child := range r.requirements {
		if rule := child.FindRule(slot, name); rule != nil {
			return rule
		}
	}
	return nil
}

func (r *AndRequirement) Rules() []*Rule {
	var rules []*Rule
	for _, child := range r.requirements {
		rules = append(rules, child.Rules()...)
	}
	return rules
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
	case approved > 0 && pending == 0:
		status = common.StatusApproved
		description = fmt.Sprintf("All rules are approved")
	case pending > 0:
		status = common.StatusPending
		description = fmt.Sprintf("%d/%d rules approved", approved, approved+pending)
	}

	return common.Result{
		Name:              "and",
		Status:            status,
		StatusDescription: description,
		Error:             err,
		Children:          children,
	}
}
