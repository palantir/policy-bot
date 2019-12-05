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
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
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

	if len(missingResults) == 1 {
		return false, fmt.Sprintf("%q has not been executed on this commit", missingResults[0]), nil
	} else if len(missingResults) > 1 {
		missingResultsStr := strings.Join(missingResults, ",")
		return false, fmt.Sprintf("%q have not been executed on this commit", missingResultsStr), nil
	}

	if len(failingStatuses) == 1 {
		return false, fmt.Sprintf("%q has not passed", failingStatuses[0]), nil
	} else if len(failingStatuses) > 1 {
		failingStatusesStr := strings.Join(failingStatuses, ",")
		return false, fmt.Sprintf("The statuses %q have not passed", failingStatusesStr), nil
	}
	return true, "", nil
}
