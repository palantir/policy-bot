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
	"math/rand"
	"strings"

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
			if c.Status == common.StatusPending {
				r = append(r, findLeafChildren(*c)...)
			}
		}
	}
	return r
}

// select n random values from the list of users without reuse
func selectRandomUsers(n int, users []string, r *rand.Rand) []string {
	selected := make(map[int]bool)

	var selections []string
	if n == 0 {
		return selections
	}
	if n >= len(users) {
		return users
	}

	for i := 0; i < n; i++ {
		for {
			i := r.Intn(len(users))
			if !selected[i] {
				selected[i] = true
				selections = append(selections, users[i])
				break
			}
		}
	}
	return selections
}

func shoveIntoMap(m map[string]struct{}, u []string) map[string]struct{} {
	for _, n := range u {
		m[n] = struct{}{}
	}
	return m
}

func selectTeamMembers(prctx pull.Context, allTeams []string, r *rand.Rand) ([]string, error) {
	randomTeam := allTeams[r.Intn(len(allTeams))]
	teamInfo := strings.Split(randomTeam, "/")
	teamMembers, err := prctx.ListTeamMembers(teamInfo[0], teamInfo[1])
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to get member listing for team %s", randomTeam)
	}
	return teamMembers, nil
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, r *rand.Rand) ([]string, error) {
	logger := zerolog.Ctx(ctx)
	pendingLeafNodes := findLeafChildren(result)
	var requestedUsers []string

	logger.Debug().Msgf("Collecting reviewers for %d pending leaf nodes", len(pendingLeafNodes))

	for _, child := range pendingLeafNodes {
		allUsers := make(map[string]struct{})
		allUsers = shoveIntoMap(allUsers, child.Rule.RequestedUsers)

		if len(child.Rule.RequestedTeams) > 0 {
			teamMembers, err := selectTeamMembers(prctx, child.Rule.RequestedTeams, r)
			if err != nil {
				logger.Warn().Err(err).Msgf("Unable to get member listing for teams, skipping team member selection")
			}
			allUsers = shoveIntoMap(allUsers, teamMembers)
		}

		if len(child.Rule.RequestedOrganizations) > 0 {
			randomOrg := child.Rule.RequestedOrganizations[r.Intn(len(child.Rule.RequestedOrganizations))]
			orgMembers, err := prctx.ListOrganizationMembers(randomOrg)
			if err != nil {
				logger.Warn().Err(err).Msgf("Unable to get member listing for org %s, skipping org member selection", randomOrg)
			}
			allUsers = shoveIntoMap(allUsers, orgMembers)
		}

		allCollaborators, err := prctx.ListRepositoryCollaborators()
		if err != nil {
			return nil, errors.Wrap(err, "Unable to list repository collaborators")
		}
		collaboratorPermissions := make(map[string]string)

		// TODO(asvoboda):
		// Replace the expensive permission checking with graphql calls: https://developer.github.com/v4/object/repositorycollaboratoredge/
		for _, c := range allCollaborators {
			perm, err := prctx.CollaboratorPermission(prctx.RepositoryOwner(), prctx.RepositoryName(), c)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to determine permission level of %s on repo %s", c, prctx.RepositoryName())
			}
			collaboratorPermissions[c] = perm
		}

		if child.Rule.RequestedAdmins {
			var repoAdmins []string
			for _, c := range allCollaborators {
				if collaboratorPermissions[c] == "admin" {
					repoAdmins = append(repoAdmins, c)
				}
			}
			allUsers = shoveIntoMap(allUsers, repoAdmins)
		}

		if child.Rule.RequestedWriteCollaborators {
			var repoCollaborators []string
			for _, c := range allCollaborators {
				if collaboratorPermissions[c] == "write" {
					repoCollaborators = append(repoCollaborators, c)
				}
			}
			allUsers = shoveIntoMap(allUsers, repoCollaborators)
		}

		// Remove author before randomly selecting, since github will fail to assign _anyone_
		if _, ok := allUsers[prctx.Author()]; ok {
			delete(allUsers, prctx.Author())
		}

		// Remove any users who aren't collaborators, since github will fail to assign _anyone_
		for k := range allUsers {
			if _, ok := collaboratorPermissions[k]; !ok {
				delete(allUsers, k)
			}
		}

		var allUserList []string
		for k := range allUsers {
			allUserList = append(allUserList, k)
		}

		logger.Debug().Msgf("Found %d total candidates for review after removing author and non-collaborators; randomly selecting some", len(allUsers))
		randomSelection := selectRandomUsers(child.Rule.RequiredCount, allUserList, r)
		requestedUsers = append(requestedUsers, randomSelection...)
	}

	return requestedUsers, nil
}
