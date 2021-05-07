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
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasSuccessfulStatus []string

var _ Predicate = HasSuccessfulStatus([]string{})

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list commit statuses")
	}

	var missingResults []string
	var failingStatuses []string
	for _, status := range pred {
		result, ok := statuses[status]
		if !ok {
			missingResults = append(missingResults, status)
		}
		if result != "success" {
			failingStatuses = append(failingStatuses, status)
		}
	}

	if len(missingResults) > 0 {
		return false, "One or more statuses is missing: " + strings.Join(missingResults, ", "), nil
	}

	if len(failingStatuses) > 0 {
		return false, "One or more statuses has not passed: " + strings.Join(failingStatuses, ","), nil
	}
	return true, "", nil
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}
