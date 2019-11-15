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

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
)

type HasSuccessfulStatus string

var _ Predicate = HasSuccessfulStatus("")

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list commit statuses")
	}

	status, ok := statuses[string(pred)]
	if !ok {
		return false, fmt.Sprintf("%q has not been executed on this commit", pred), nil
	}
	if status == "success" {
		return true, "", nil
	}

	return false, fmt.Sprintf("%q has not passed", pred), nil
}
