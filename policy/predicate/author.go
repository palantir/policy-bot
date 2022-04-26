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
	"sort"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasAuthorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasAuthorIn{}

func (pred *HasAuthorIn) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	author := prctx.Author()

	result, err := pred.IsActor(ctx, prctx, author)
	desc := ""
	if !result {
		desc = fmt.Sprintf("The pull request author %q does not meet the required membership conditions", author)
	}

	predicateResult := common.PredicateResult{
		Satisfied:       result,
		Description:     desc,
		ValuePhrase:     "authors",
		Values:          []string{author},
		ConditionPhrase: "meet the required membership conditions",
		ConditionsMap: map[string][]string{
			"Organizations": pred.Organizations,
			"Teams":         pred.Teams,
			"Users":         pred.Users,
		},
	}
	return &predicateResult, err
}

func (pred *HasAuthorIn) Trigger() common.Trigger {
	return common.TriggerStatic
}

type OnlyHasContributorsIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &OnlyHasContributorsIn{}

func (pred *OnlyHasContributorsIn) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ValuePhrase:     "contributors",
		ConditionPhrase: "all meet the required membership conditions",
		ConditionsMap: map[string][]string{
			"Organizations": pred.Organizations,
			"Teams":         pred.Teams,
			"Users":         pred.Users,
		},
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	users := make(map[string]struct{})
	users[prctx.Author()] = struct{}{}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
	}

	userList := make([]string, 0, len(users))
	for user := range users {
		userList = append(userList, user)
	}
	sort.Strings(userList)

	for _, user := range userList {
		member, err := pred.IsActor(ctx, prctx, user)
		if err != nil {
			return nil, err
		}
		if !member {
			predicateResult.Description = fmt.Sprintf("Contributor %q does not meet the required membership conditions", user)
			predicateResult.Values = []string{user}
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}
	predicateResult.Values = userList
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred *OnlyHasContributorsIn) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasContributorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasContributorIn{}

func (pred *HasContributorIn) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ValuePhrase:     "contributors",
		ConditionPhrase: "meet the required membership conditions ",
		ConditionsMap: map[string][]string{
			"Organizations": pred.Organizations,
			"Teams":         pred.Teams,
			"Users":         pred.Users,
		},
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	users := make(map[string]struct{})
	users[prctx.Author()] = struct{}{}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
	}

	userList := make([]string, 0, len(users))
	for user := range users {
		userList = append(userList, user)
	}
	sort.Strings(userList)

	for _, user := range userList {
		member, err := pred.IsActor(ctx, prctx, user)
		if err != nil {
			return nil, err
		}
		if member {
			predicateResult.Satisfied = true
			predicateResult.Values = []string{user}
			return &predicateResult, nil
		}
	}
	predicateResult.Description = "No contributors meet the required membership conditions"
	predicateResult.Satisfied = false
	predicateResult.Values = userList
	return &predicateResult, nil
}

func (pred *HasContributorIn) Trigger() common.Trigger {
	return common.TriggerCommit
}

type AuthorIsOnlyContributor bool

var _ Predicate = AuthorIsOnlyContributor(false)

func (pred AuthorIsOnlyContributor) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ValuePhrase:     "authors",
		ConditionPhrase: "meet the condition",
	}
	if pred {
		predicateResult.ConditionValues = []string{"they are the only contributors"}
	} else {
		predicateResult.ConditionValues = []string{"they are not the only contributors"}
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	author := prctx.Author()
	predicateResult.Values = []string{author}

	for _, c := range commits {
		if c.Author != author || (!c.CommittedViaWeb && c.Committer != author) {
			if pred {
				predicateResult.Description = fmt.Sprintf("Commit %.10s was authored or committed by a different user", c.SHA)
				predicateResult.Satisfied = false
				return &predicateResult, nil
			}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}

	if pred {
		predicateResult.Satisfied = true
		return &predicateResult, nil
	}
	predicateResult.Description = fmt.Sprintf("All commits were authored and committed by %s", author)
	predicateResult.Satisfied = false
	return &predicateResult, nil
}

func (pred AuthorIsOnlyContributor) Trigger() common.Trigger {
	return common.TriggerCommit
}
