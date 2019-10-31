// Copyright 2019 Palantir Technologies, Inc.
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

package reviewer

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

const (
	LogKeyLeafNode = "leaf_node"
)

func findLeafChildren(result common.Result) []common.Result {
	var r []common.Result
	if len(result.Children) == 0 {
		if result.Status == common.StatusPending && result.Error == nil {
			return []common.Result{result}
		}
	} else {
		for _, c := range result.Children {
			if c.Status == common.StatusPending {
				r = append(r, findLeafChildren(*c)...)
			}
		}
	}
	return r
}

// select n random values from the list of users without reuse
func selectRandomUsers(n int, users []string, r *rand.Rand) []string {
	var selections []string
	if n == 0 {
		return selections
	}
	if n >= len(users) {
		return users
	}

	selected := make(map[int]bool)
	for i := 0; i < n; i++ {
		j := 0
		for {
			// Upper bound the number of attempts to uniquely select random users to n*5
			if j > n*5 {
				// We haven't been able to select a random value, bail loudly
				panic(fmt.Sprintf("failed to select random value for %d %d", n, len(users)))
			}
			m := r.Intn(len(users))
			if !selected[m] {
				selected[m] = true
				selections = append(selections, users[m])
				break
			}
			j++
		}
	}
	return selections
}

func selectTeamMembers(prctx pull.Context, allTeams []string) ([]string, error) {
	var allTeamsMembers []string
	for _, team := range allTeams {
		teamMembers, err := prctx.TeamMembers(team)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get member listing for team %s", team)
		}
		allTeamsMembers = append(allTeamsMembers, teamMembers...)
	}

	return allTeamsMembers, nil
}

func selectOrgMembers(prctx pull.Context, allOrgs []string) ([]string, error) {
	var allOrgsMembers []string
	for _, org := range allOrgs {
		orgMembers, err := prctx.OrganizationMembers(org)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get member listing for org %s", org)
		}
		allOrgsMembers = append(allOrgsMembers, orgMembers...)
	}

	return allOrgsMembers, nil
}

func selectAdmins(prctx pull.Context) ([]string, error) {
	// Resolve all admin teams on the repo, and resolve their user membership
	var adminUsers []string
	teams, err := prctx.Teams()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get list of teams collaborators")
	}

	for team, perm := range teams {
		if perm == common.GithubAdminPermission {
			fullTeamName := fmt.Sprintf("%s/%s", prctx.RepositoryOwner(), team)
			admins, err := prctx.TeamMembers(fullTeamName)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get list of members for %s", team)
			}
			adminUsers = append(adminUsers, admins...)
		}
	}

	return adminUsers, nil
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, r *rand.Rand) ([]string, error) {
	logger := *zerolog.Ctx(ctx)
	var usersToRequest []string
	pendingLeafNodes := findLeafChildren(result)

	logger.Debug().Msgf("Found %d pending leaf nodes for review selection", len(pendingLeafNodes))
	for _, child := range pendingLeafNodes {
		logger = logger.With().Str(LogKeyLeafNode, child.Name).Logger()

		allUsers := make(map[string]struct{})

		for _, user := range child.ReviewRequestRule.Users {
			allUsers[user] = struct{}{}
		}

		if len(child.ReviewRequestRule.Teams) > 0 {
			logger.Debug().Msg("Selecting from teams for review")
			teamMembers, err := selectTeamMembers(prctx, child.ReviewRequestRule.Teams)
			if err != nil {
				logger.Warn().Err(err).Msgf("failed to get member listing for teams, skipping team member selection")
			}
			for _, user := range teamMembers {
				allUsers[user] = struct{}{}
			}
		}

		if len(child.ReviewRequestRule.Organizations) > 0 {
			logger.Debug().Msg("Selecting from organizations for review")
			orgMembers, err := selectOrgMembers(prctx, child.ReviewRequestRule.Organizations)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to get member listing for org, skipping org member selection")
			}
			for _, user := range orgMembers {
				allUsers[user] = struct{}{}
			}
		}

		collaboratorsToConsider, err := prctx.RepositoryCollaborators()
		if err != nil {
			return nil, errors.Wrap(err, "failed to list repository collaborators")
		}

		if child.ReviewRequestRule.WriteCollaborators {
			logger.Debug().Msg("Selecting from write collaborators for review")
			for user, perm := range collaboratorsToConsider {
				if perm == common.GithubWritePermission {
					allUsers[user] = struct{}{}
				}
			}
		}

		if child.ReviewRequestRule.Admins {
			logger.Debug().Msg("Selecting from admins for review")
			admins, err := selectAdmins(prctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to select admins")
			}

			for _, admin := range admins {
				allUsers[admin] = struct{}{}
			}
		}

		var possibleReviewers []string
		for u := range allUsers {
			// Remove the author and any users who aren't collaborators
			// since github will fail to assign _anyone_ if the request contains one of these
			_, ok := collaboratorsToConsider[u]
			if u != prctx.Author() && ok {
				possibleReviewers = append(possibleReviewers, u)
			}
		}

		if len(possibleReviewers) > 0 {
			logger.Debug().Msgf("Found %d total candidates for review after removing author and non-collaborators; randomly selecting %d", len(possibleReviewers), child.ReviewRequestRule.RequiredCount)
			randomSelection := selectRandomUsers(child.ReviewRequestRule.RequiredCount, possibleReviewers, r)
			usersToRequest = append(usersToRequest, randomSelection...)
		} else {
			logger.Debug().Msg("Did not find candidates for review after removing author and non-collaborators")
		}
	}

	return usersToRequest, nil
}
