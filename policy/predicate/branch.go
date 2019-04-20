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
	"regexp"

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
)

type TargetsBranch struct {
	Pattern string `yaml:"pattern"`
}

var _ Predicate = &TargetsBranch{}

func (pred *TargetsBranch) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	pattern, err := regexp.Compile(pred.Pattern)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to compile the target regex")
	}

	targetName, _ := prctx.Branches()
	matches := pattern.MatchString(targetName)

	desc := ""
	if !matches {
		desc = fmt.Sprintf("Target branch %q does not match required pattern %q", targetName, pred.Pattern)
	}

	return matches, desc, nil
}
