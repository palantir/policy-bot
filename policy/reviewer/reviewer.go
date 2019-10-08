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

func findLeafChildren(result common.Result) []common.Result {
	var r []common.Result
	if len(result.Children) == 0 {
		if result.Status == common.StatusPending && result.Error == nil {
			return []common.Result{result}
		}
	} else {
		for _, c := range result.Children {
			if c == nil {
				continue
			}
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
				panic(fmt.Sprintf("Unable to select random value for %d %d", n, len(users)))
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

func shoveIntoMap(m map[string]struct{}, u []string) {
	for _, n := range u {
		m[n] = struct{}{}
	}
}

func selectTeamMembers(prctx pull.Context, allTeams []string, r *rand.Rand) ([]string, error) {
	randomTeam := allTeams[r.Intn(len(allTeams))]
	teamMembers, err := prctx.TeamMembers(randomTeam)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get member listing for team %s", randomTeam)
	}
	return teamMembers, nil
}

func selectOrgMembers(prctx pull.Context, allOrgs []string, r *rand.Rand) ([]string, error) {
	randomOrg := allOrgs[r.Intn(len(allOrgs))]
	orgMembers, err := prctx.OrganizationMembers(randomOrg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get member listing for org %s", randomOrg)
	}
	return orgMembers, nil
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, r *rand.Rand) ([]string, error) {
	logger := zerolog.Ctx(ctx)
	pendingLeafNodes := findLeafChildren(result)
	var requestedUsers []string

	logger.Debug().Msgf("Collecting reviewers for %d pending leaf nodes", len(pendingLeafNodes))

	for _, child := range pendingLeafNodes {
		allUsers := make(map[string]struct{})
		shoveIntoMap(allUsers, child.ReviewRequestRule.Users)

		if len(child.ReviewRequestRule.Teams) > 0 {
			teamMembers, err := selectTeamMembers(prctx, child.ReviewRequestRule.Teams, r)
			if err != nil {
				logger.Warn().Err(err).Msgf("Unable to get member listing for teams, skipping team member selection")
			}
			shoveIntoMap(allUsers, teamMembers)
		}

		if len(child.ReviewRequestRule.Organizations) > 0 {
			orgMembers, err := selectOrgMembers(prctx, child.ReviewRequestRule.Organizations, r)
			if err != nil {
				logger.Warn().Err(err).Msg("Unable to get member listing for org, skipping org member selection")
			}
			shoveIntoMap(allUsers, orgMembers)
		}

		allCollaboratorPermissions := make(map[string]string)
		allCollaboratorAndPermissions, err := prctx.RepositoryCollaborators()
		if err != nil {
			return nil, errors.Wrap(err, "Unable to list repository collaborators")
		}

		if child.ReviewRequestRule.WriteCollaborators {
			var repoCollaborators []string
			for user, perm := range allCollaboratorAndPermissions {
				if perm == common.GithubWritePermission {
					repoCollaborators = append(repoCollaborators, user)
				}
			}
			shoveIntoMap(allUsers, repoCollaborators)
		}

		// When admins are selected for review, only collect the desired set of admins instead of
		// everyone, which includes org admins
		if !child.ReviewRequestRule.Admins {
			// When not looking for admins, we want to check with everyone
			allCollaboratorPermissions = allCollaboratorAndPermissions
		} else {
			// Determine what the scope of requested admins should be
			switch child.ReviewRequestRule.AdminScope {
			case common.User:
				logger.Debug().Msg("Selecting admin users with direct collaboration rights")
				directCollaborators, err := prctx.DirectRepositoryCollaborators()
				if err != nil {
					logger.Error().Err(err).Msgf("Unable to get list of direct collaborators on %s", prctx.RepositoryName())
				}

				var repoAdmins []string
				for user, perm := range directCollaborators {
					if perm == common.GithubAdminPermission {
						repoAdmins = append(repoAdmins, user)
					}
				}
				shoveIntoMap(allUsers, repoAdmins)
				for _, a := range repoAdmins {
					allCollaboratorPermissions[a] = common.GithubAdminPermission
				}

				break
			case common.Team:
				// Only request review for admins that are added as a team
				// Resolve all admin teams on the repo, and resolve their user membership
				logger.Debug().Msg("Selecting admin users from teams")
				teams, err := prctx.Teams()
				if err != nil {
					logger.Error().Err(err).Msg("Unable to get list of teams collaborators")
				}

				var adminUsers []string
				for team, perm := range teams {
					if perm == common.GithubAdminPermission {
						fullTeamName := fmt.Sprintf("%s/%s", prctx.RepositoryOwner(), team)
						admins, err := prctx.TeamMembers(fullTeamName)
						if err != nil {
							logger.Error().Err(err).Msgf("Unable to get list of members for %s", team)
						}
						adminUsers = append(adminUsers, admins...)
					}
				}

				shoveIntoMap(allUsers, adminUsers)
				for _, a := range adminUsers {
					allCollaboratorPermissions[a] = common.GithubAdminPermission
				}

				break
			case common.Org:
				logger.Debug().Msg("Selecting admin users from the org")
				orgOwners, err := prctx.OrganizationOwners(prctx.RepositoryOwner())
				if err != nil {
					logger.Error().Err(err).Msgf("Unable to get list of org owners for %s", prctx.RepositoryOwner())
				}

				for _, o := range orgOwners {
					allUsers[o] = struct{}{}
					allCollaboratorPermissions[o] = common.GithubAdminPermission
				}
				break
			default:
				// unknown option, log error and don't make any assumptions
				logger.Warn().Msgf("Unknown AdminScope %s, ignoring", child.ReviewRequestRule.AdminScope)
			}
		}

		var allUserList []string
		for u := range allUsers {
			// Remove the author and any users who aren't collaborators
			// since github will fail to assign _anyone_ if the request contains one of these
			_, ok := allCollaboratorPermissions[u]
			if u != prctx.Author() && ok {
				allUserList = append(allUserList, u)
			}
		}

		logger.Debug().Msgf("Found %d total candidates for review after removing author and non-collaborators; randomly selecting %d", len(allUserList), child.ReviewRequestRule.RequiredCount)
		randomSelection := selectRandomUsers(child.ReviewRequestRule.RequiredCount, allUserList, r)
		requestedUsers = append(requestedUsers, randomSelection...)
	}

	return requestedUsers, nil
}
