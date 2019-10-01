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
	"time"

	"github.com/google/go-github/github"
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

func listAllTeamMembers(ctx context.Context, client *github.Client, team *github.Team) ([]string, error) {
	opt := &github.TeamListTeamMembersOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	// get all pages of results
	var allUsers []string

	for {
		users, resp, err := client.Teams.ListTeamMembers(ctx, team.GetID(), opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list team %s members page %d", team.GetName(), opt.Page)
		}
		for _, u := range users {
			allUsers = append(allUsers, u.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

func listAllOrgMembers(ctx context.Context, client *github.Client, org string) ([]string, error) {
	opt := &github.ListMembersOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	// get all pages of results
	var allUsers []string

	for {
		users, resp, err := client.Organizations.ListMembers(ctx, org, opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list members of org %s page %d", org, opt.Page)
		}
		for _, u := range users {
			allUsers = append(allUsers, u.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

func listAllCollaborators(ctx context.Context, client *github.Client, org, repo, desiredPerm string) ([]string, error) {
	opt := &github.ListCollaboratorsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	// get all pages of results
	var allUsers []string

	for {
		users, resp, err := client.Repositories.ListCollaborators(ctx, org, repo, opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list members of org %s page %d", org, opt.Page)
		}
		for _, u := range users {
			perm, _, err := client.Repositories.GetPermissionLevel(ctx, org, repo, u.GetLogin())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to determine permission level of %s on repo %s", u.GetLogin(), repo)
			}
			if perm.GetPermission() == desiredPerm {
				allUsers = append(allUsers, u.GetLogin())
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

// select n random values from the list of users, never selecting the blacklist
func selectRandomUsers(n int, users []string, blacklist string) []string {
	generated := map[int]struct{}{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	var selections []string
	if n == 0 {
		return selections
	}
	if n == 1 && len(users) == 1 && users[0] == blacklist {
		return selections
	}
	if n > len(users) {
		return users
	}

	for i := 0; i < n; i++ {
		for {
			i := r.Intn(len(users))
			if _, ok := generated[i]; !ok {
				generated[i] = struct{}{}
				if users[i] != blacklist {
					selections = append(selections, users[i])
					break
				}
			}
		}
	}
	return selections
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, client *github.Client) ([]string, error) {
	logger := zerolog.Ctx(ctx)
	// look for requested reviewers
	pendingLeafNodes := findLeafChildren(result)
	var requestedUsers []string
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	logger.Debug().Msgf("Collecting reviewers for %d pending leaf nodes", len(pendingLeafNodes))

	for _, child := range pendingLeafNodes {

		selectionType := r.Intn(5)

		logger.Debug().Msgf("Picking %d", selectionType)

		var randomAssignees []string
		switch selectionType {
		case 0:
			// pick a user
			randomAssignees = selectRandomUsers(child.RequiredCount, child.RequestedUsers, prctx.Author())
			logger.Debug().Str("users", strings.Join(randomAssignees, ",")).Msg("Select from set of hardcoded usernames")
		case 1:
			// pick a user from a team
			randomTeam := child.RequestedTeams[r.Intn(len(child.RequestedTeams))]
			teamInfo := strings.Split(randomTeam, "/")
			team, _, err := client.Teams.GetTeamBySlug(ctx, teamInfo[0], teamInfo[1])
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get information for team %s", randomTeam)
			}

			teamMembers, err := listAllTeamMembers(ctx, client, team)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get member listing for team %s", randomTeam)
			}

			randomAssignees = selectRandomUsers(child.RequiredCount, teamMembers, prctx.Author())
			logger.Debug().Str("user", strings.Join(randomAssignees, ",")).Str("team", randomTeam).Msg("Select from set of teams")
		case 2:
			// pick a user from an org
			randomOrg := child.RequestedOrganizations[r.Intn(len(child.RequestedOrganizations))]
			orgMembers, err := listAllOrgMembers(ctx, client, randomOrg)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get member listing for org %s", randomOrg)
			}
			randomAssignees = selectRandomUsers(child.RequiredCount, orgMembers, prctx.Author())
			logger.Debug().Str("user", strings.Join(randomAssignees, ",")).Str("org", randomOrg).Msg("Select from set of orgs")
		case 3:
			// pick a user who is an admin of the repo
			repoAdmins, err := listAllCollaborators(ctx, client, prctx.RepositoryOwner(), prctx.RepositoryName(), "admin")
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get admin listing")
			}

			randomAssignees = selectRandomUsers(child.RequiredCount, repoAdmins, prctx.Author())
			logger.Debug().Str("user", strings.Join(randomAssignees, ",")).Msg("Select from set of admins")
		case 4:
			// pick a user who is a write collaborator on the repo
			repoCollaborators, err := listAllCollaborators(ctx, client, prctx.RepositoryOwner(), prctx.RepositoryName(), "write")
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get admin listing")
			}
			randomAssignees = selectRandomUsers(child.RequiredCount, repoCollaborators, prctx.Author())
			logger.Debug().Str("user", strings.Join(randomAssignees, ",")).Msg("Select from set of write collaborators")
		}
		requestedUsers = append(requestedUsers, randomAssignees...)
	}

	// remove PR author if they exist in the possible list, since they cannot be assigned to their own PRs
	for i, u := range requestedUsers {
		if u == prctx.Author() {
			requestedUsers = append(requestedUsers[:i], requestedUsers[i+1:]...)
		}
	}

	return requestedUsers, nil
}
