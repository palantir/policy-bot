// Copyright 2020 Palantir Technologies, Inc.
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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

// HasLabels checks that the pull request has all of the specified labels.
//
// Deprecated: use the more flexible `Labels` with `has_all` instead.
type HasLabels []string

var _ Predicate = HasLabels([]string{})

func (pred HasLabels) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	return Labels{HasAll: pred}.Evaluate(ctx, prctx)
}

func (pred HasLabels) Trigger() common.Trigger {
	return common.TriggerLabel
}

func contains(elements []string, value string) bool {
	return slices.Contains(elements, value)
}
