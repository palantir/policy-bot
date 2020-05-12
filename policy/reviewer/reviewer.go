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
	"strings"

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

func selectTeamMembers(prctx pull.Context, allTeams []string) (map[string][]string, error) {
	var allTeamsMembers = make(map[string][]string)
	for _, team := range allTeams {
		teamMembers, err := prctx.TeamMembers(team)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get member listing for team %s", team)
		}
		allTeamsMembers[team] = teamMembers
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

func selectAdminTeamMembers(prctx pull.Context) (map[string][]string, error) {
	// Resolve all admin teams on the repo, and resolve their user membership
	adminTeams := make(map[string][]string)
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
			adminTeams[fullTeamName] = admins
		}
	}

	return adminTeams, nil
}

func getPossibleReviewers(prctx pull.Context, users map[string]struct{}, collaboratorsToConsider map[string]string) []string {
	var possibleReviewers []string
	for u := range users {
		// Remove the author and any users who aren't collaborators
		// since github will fail to assign _anyone_ if the request contains one of these
		_, ok := collaboratorsToConsider[u]
		if u != prctx.Author() && ok {
			possibleReviewers = append(possibleReviewers, u)
		}
	}
	return possibleReviewers
}

func stripOrg(teamWithOrg string) (string, error) {
	split := strings.Split(teamWithOrg, "/")
	if len(split) < 0 {
		return "", fmt.Errorf("Expected org with team: '%s'", teamWithOrg)
	}
	return split[1], nil
}

func SelectReviewers(ctx context.Context, prctx pull.Context, result common.Result, r *rand.Rand) ([]string, []string, error) {
	usersToRequest := make([]string, 0)
	teamsToRequest := make([]string, 0)
	pendingLeafNodes := findLeafChildren(result)
	zerolog.Ctx(ctx).Debug().Msgf("Found %d pending leaf nodes for review selection", len(pendingLeafNodes))

	for _, child := range pendingLeafNodes {
		logger := zerolog.Ctx(ctx).With().Str(LogKeyLeafNode, child.Name).Logger()

		allUsers := make(map[string]struct{})
		allTeamsWithUsers := make(map[string][]string)

		for _, user := range child.ReviewRequestRule.Users {
			allUsers[user] = struct{}{}
		}

		if len(child.ReviewRequestRule.Teams) > 0 {
			logger.Debug().Msg("Selecting from teams for review")
			teamsToUsers, err := selectTeamMembers(prctx, child.ReviewRequestRule.Teams)
			if err != nil {
				logger.Warn().Err(err).Msgf("failed to get member listing for teams, skipping team member selection")
			}
			for team, users := range teamsToUsers {
				allTeamsWithUsers[team] = users
				for _, user := range users {
					allUsers[user] = struct{}{}
				}
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
			return nil, nil, errors.Wrap(err, "failed to list repository collaborators")
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
			adminTeams, err := selectAdminTeamMembers(prctx)
			if err != nil {
				return nil, nil, errors.Wrap(err, "failed to select admins")
			}
			for team, members := range adminTeams {
				allTeamsWithUsers[team] = members
				for _, admin := range members {
					allUsers[admin] = struct{}{}
				}
			}
		}

		switch child.GetMode() {
		case common.RequestModeTeams:
			var possibleTeams []string
			permissionedTeams, err := prctx.Teams()
			if err != nil {
				return nil, nil, err
			}
			for team := range allTeamsWithUsers {
				formattedTeam, err := stripOrg(team)
				if err != nil {
					return nil, nil, err
				}
				if _, ok := permissionedTeams[formattedTeam]; ok {
					possibleTeams = append(possibleTeams, team)
				}
			}
			logger.Debug().Msgf("Found %d total teams; requesting teams", len(possibleTeams))
			teamsToRequest = append(teamsToRequest, possibleTeams...)
		case common.RequestModeAllUsers:
			possibleReviewers := getPossibleReviewers(prctx, allUsers, collaboratorsToConsider)
			if len(possibleReviewers) > 0 {
				logger.Debug().Msgf("Found %d total reviewers after removing author and non-collaborators; requesting all", len(possibleReviewers))
				usersToRequest = append(usersToRequest, possibleReviewers...)
			} else {
				logger.Debug().Msg("Did not find candidates for review after removing author and non-collaborators")
			}
		case common.RequestModeRandomUsers:
			possibleReviewers := getPossibleReviewers(prctx, allUsers, collaboratorsToConsider)
			if len(possibleReviewers) > 0 {
				logger.Debug().Msgf("Found %d total candidates for review after removing author and non-collaborators; randomly selecting %d", len(possibleReviewers), child.ReviewRequestRule.RequiredCount)
				randomSelection := selectRandomUsers(child.ReviewRequestRule.RequiredCount, possibleReviewers, r)
				usersToRequest = append(usersToRequest, randomSelection...)
			} else {
				logger.Debug().Msg("Did not find candidates for review after removing author and non-collaborators")
			}
		default:
			return nil, nil, fmt.Errorf("unknown mode '%s' supplied", child.ReviewRequestRule.Mode)
		}
	}

	return usersToRequest, teamsToRequest, nil
}
