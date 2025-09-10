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

package predicate

import (
	"context"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

// HasStatus checks that the specified statuses have a completed status with configurable conclusions.
//
// Deprecated: use the more flexible `HasStatusCheck` instead.
type HasStatus struct {
	Conclusions []string `yaml:"conclusions,omitempty"`
	Statuses    []string `yaml:"statuses,omitempty"`
}

func NewHasStatus(statuses []string, conclusions []string) *HasStatus {
	return &HasStatus{
		Conclusions: conclusions,
		Statuses:    statuses,
	}
}

var _ Predicate = HasStatus{}

func (pred HasStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	// Convert strings into a regex object to comply with function signature
	var checks []common.Regexp
	for _, status := range pred.Statuses {
		check, err := common.NewRegexp(status)
		if err == nil {
			checks = append(checks, check)
		}
	}
	return HasStatusCheck{Checks: checks, Conclusions: pred.Conclusions, noRegex: true}.Evaluate(ctx, prctx)
}

func (pred HasStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// HasSuccessfulStatus checks that the specified statuses have a successful
// conclusion.
//
// Deprecated: use the more flexible `HasStatusCheck` with `conclusions: ["success"]`
// instead.
type HasSuccessfulStatus []string

var _ Predicate = HasSuccessfulStatus{}

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	return HasStatus{Statuses: pred}.Evaluate(ctx, prctx)
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}
