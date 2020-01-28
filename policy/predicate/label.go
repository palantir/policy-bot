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
	"strings"

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
)

type HasLabels []string

var _ Predicate = HasLabels([]string{})

func (pred HasLabels) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	if len(pred) > 0 {
		labels, err := prctx.Labels()
		if err != nil {
			return false, "", errors.Wrap(err, "failed to list pull request labels")
		}

		for _, requiredLabel := range pred {
			if !contains(labels, strings.ToLower(requiredLabel)) {
				return false, "Missing label: " + requiredLabel, nil
			}
		}
	}

	return true, "", nil
}

func contains(elements []string, value string) bool {
	for _, element := range elements {
		if element == value {
			return true
		}
	}
	return false
}
