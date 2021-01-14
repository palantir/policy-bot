// Copyright 2021 Palantir Technologies, Inc.
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

type Title struct {
	Matches    []common.Regexp `yaml:"matches"`
	NotMatches []common.Regexp `yaml:"not_matches"`
}

var _ Predicate = Title{}

func (pred Title) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	title := prctx.Title()

	if len(pred.Matches) > 0 {
		if !anyMatches(pred.Matches, title) {
			return false, "Title doesn't match a required pattern", nil
		}
	}

	if len(pred.NotMatches) > 0 {
		if anyMatches(pred.NotMatches, title) {
			return false, "Title matches a disallowed pattern", nil
		}
	}

	return true, "", nil
}

func (pred Title) Trigger() common.Trigger {
	return common.TriggerPullRequest
}
