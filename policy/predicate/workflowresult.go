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

// HasWorkflowResult checks that the specified workflow has a configurable conclusion.
//
// Deprecated: use the more flexible `HasWorkflow` instead.
type HasWorkflowResult struct {
	Conclusions AllowedConclusions `yaml:"conclusions,omitempty"`
	Workflows   []string           `yaml:"workflows,omitempty"`
}

func NewHasWorkflowResult(workflows []string, conclusions []string) *HasWorkflowResult {
	return &HasWorkflowResult{
		Conclusions: conclusions,
		Workflows:   workflows,
	}
}

var _ Predicate = HasWorkflowResult{}

func (pred HasWorkflowResult) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	regexpWorkflows := make([]common.Regexp, len(pred.Workflows))
	for i, workflow := range pred.Workflows {
		regexpWorkflows[i], _ = common.NewRegexp(workflow)
	}
	return HasWorkflow{Workflows: regexpWorkflows, Conclusions: pred.Conclusions}.Evaluate(ctx, prctx)
}

func (pred HasWorkflowResult) Trigger() common.Trigger {
	return common.TriggerStatus
}
