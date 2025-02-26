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
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type AllowedConclusions []string
type AllowedStatuses []string

// HasStatus checks that the specified statuses have a completed status with configurable conclusions.
//
// Deprecated: use the more flexible `HasStatusCheck` instead.
type HasStatus struct {
	Conclusions AllowedConclusions `yaml:"conclusions,omitempty"`
	Statuses    []string           `yaml:"statuses,omitempty"`
}

func NewHasStatus(statuses []string, conclusions []string) *HasStatus {
	return &HasStatus{
		Conclusions: conclusions,
		Statuses:    statuses,
	}
}

var _ Predicate = HasStatus{}

func (pred HasStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	return HasStatusCheck{Checks: pred.Statuses, Conclusions: pred.Conclusions}.Evaluate(ctx, prctx)
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
	return HasStatusCheck{Checks: pred}.Evaluate(ctx, prctx)
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// joinWithOr returns a string that represents the allowed conclusions in a
// format that can be used in a sentence. For example, if the allowed
// conclusions are "success" and "failure", this will return "success or
// failure". If there are more than two conclusions, the first n-1 will be
// separated by commas.
func (c AllowedConclusions) joinWithOr() string {
	slices.Sort(c)

	length := len(c)
	switch length {
	case 0:
		return ""
	case 1:
		return c[0]
	case 2:
		return c[0] + " or " + c[1]
	}

	head, tail := c[:length-1], c[length-1]

	return strings.Join(head, ", ") + ", or " + tail
}
func (c AllowedStatuses) joinWithOr() string {
	slices.Sort(c)

	length := len(c)
	switch length {
	case 0:
		return ""
	case 1:
		return c[0]
	case 2:
		return c[0] + " or " + c[1]
	}

	head, tail := c[:length-1], c[length-1]

	return strings.Join(head, ", ") + ", or " + tail
}
