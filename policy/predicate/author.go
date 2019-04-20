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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type HasAuthorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasAuthorIn{}

func (pred *HasAuthorIn) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	author := prctx.Author()

	result, err := pred.IsActor(ctx, prctx, author)
	desc := ""

	if !result {
		desc = fmt.Sprintf("The pull request author %q does not meet the required membership conditions", author)
	}

	return result, desc, err
}

type HasContributorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasContributorIn{}

func (pred *HasContributorIn) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to get commits")
	}

	users := make(map[string]struct{})
	users[prctx.Author()] = struct{}{}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
	}

	for user := range users {
		member, err := pred.IsActor(ctx, prctx, user)
		if err != nil {
			return false, "", err
		}
		if member {
			return true, "", nil
		}
	}

	desc := "No contributors meet the required membership conditions"
	return false, desc, nil
}
