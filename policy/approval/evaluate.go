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

	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type evaluator struct {
	root common.Evaluator
}

func (eval *evaluator) Trigger() common.Trigger {
	if eval.root != nil {
		return eval.root.Trigger()
	}
	return common.TriggerCommit
}

func (eval *evaluator) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	if eval.root != nil {
		res = eval.root.Evaluate(ctx, prctx)
	} else {
		zerolog.Ctx(ctx).Debug().Msg("No approval policy defined; skipping")

		res.Status = common.StatusApproved
		res.Description = "No approval policy defined"
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
		log.Debug().Msgf("rule evaluation resulted in %s:\"%s\"", result.Status, result.Description)
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
		Name:        "or",
		Status:      status,
		Description: description,
		Error:       err,
		Children:    children,
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
		Name:        "and",
		Status:      status,
		Description: description,
		Error:       err,
		Children:    children,
	}
}
