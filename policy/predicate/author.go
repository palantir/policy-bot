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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasAuthorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasAuthorIn{}

func (pred *HasAuthorIn) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	author := prctx.Author()

	result, err := pred.IsActor(ctx, prctx, author)
	desc := ""
	contributorInfo := common.ContributorInfo{
		Organizations: pred.Organizations,
		Teams:         pred.Teams,
		Users:         pred.Users,
		Author:        author,
		Contributors:  []string{},
	}
	predicateInfo := common.PredicateInfo{
		Type:            "HasAuthorIn",
		Name:            "Author",
		ContributorInfo: &contributorInfo,
	}
	if !result {
		desc = fmt.Sprintf("The pull request author %q does not meet the required membership conditions", author)
	}

	return result, desc, &predicateInfo, err
}

func (pred *HasAuthorIn) Trigger() common.Trigger {
	return common.TriggerStatic
}

type OnlyHasContributorsIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &OnlyHasContributorsIn{}

func (pred *OnlyHasContributorsIn) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}

	users := make(map[string]struct{})
	users[prctx.Author()] = struct{}{}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
	}

	var contributorInfo common.ContributorInfo

	contributorInfo.Organizations = pred.Organizations
	contributorInfo.Teams = pred.Teams
	contributorInfo.Users = pred.Users

	predicateInfo := common.PredicateInfo{
		Type:            "OnlyHasContributorsIn",
		Name:            "Contributors",
		ContributorInfo: &contributorInfo,
	}

	for user := range users {
		member, err := pred.IsActor(ctx, prctx, user)
		if err != nil {
			return false, "", nil, err
		}
		if !member {
			contributorInfo.Contributors = []string{user}
			return false, fmt.Sprintf("Contributor %q does not meet the required membership conditions", user), &predicateInfo, nil
		}
	}
	contributorInfo.Contributors = getKeyValues(users)
	return true, "", &predicateInfo, nil
}

func (pred *OnlyHasContributorsIn) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasContributorIn struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasContributorIn{}

func (pred *HasContributorIn) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}

	users := make(map[string]struct{})
	users[prctx.Author()] = struct{}{}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
	}

	var contributorInfo common.ContributorInfo

	contributorInfo.Organizations = pred.Organizations
	contributorInfo.Teams = pred.Teams
	contributorInfo.Users = pred.Users

	predicateInfo := common.PredicateInfo{
		Type:            "HasContributorsIn",
		Name:            "Contributors",
		ContributorInfo: &contributorInfo,
	}

	for user := range users {
		member, err := pred.IsActor(ctx, prctx, user)
		if err != nil {
			return false, "", nil, err
		}
		if member {
			contributorInfo.Contributors = []string{user}
			return true, "", &predicateInfo, nil
		}
	}
	contributorInfo.Contributors = getKeyValues(users)
	desc := "No contributors meet the required membership conditions"
	return false, desc, &predicateInfo, nil
}

func (pred *HasContributorIn) Trigger() common.Trigger {
	return common.TriggerCommit
}

type AuthorIsOnlyContributor bool

var _ Predicate = AuthorIsOnlyContributor(false)

func (pred AuthorIsOnlyContributor) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}

	author := prctx.Author()

	users := make(map[string]struct{})

	var contributorInfo common.ContributorInfo

	contributorInfo.Author = author

	predicateInfo := common.PredicateInfo{
		Type:            "AuthorIsOnlyContributor",
		Name:            "Author",
		ContributorInfo: &contributorInfo,
	}

	for _, c := range commits {
		for _, u := range c.Users() {
			users[u] = struct{}{}
		}
		if c.Author != author || (!c.CommittedViaWeb && c.Committer != author) {
			if pred {
				return false, fmt.Sprintf("Commit %.10s was authored or committed by a different user", c.SHA), &predicateInfo, nil
			}
			return true, "", &predicateInfo, nil
		}
	}

	if pred {
		return true, "", &predicateInfo, nil
	}
	return false, fmt.Sprintf("All commits were authored and committed by %s", author), &predicateInfo, nil
}

func (pred AuthorIsOnlyContributor) Trigger() common.Trigger {
	return common.TriggerCommit
}

func getKeyValues(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
