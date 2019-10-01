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

func listAllCollaborators(ctx context.Context, client *github.Client, org, repo string) ([]string, error) {
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
			allUsers = append(allUsers, u.GetLogin())
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

// select n random values from the list of users
func selectRandomUsers(n int, users []string) []string {
	generated := map[int]struct{}{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

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
			if _, ok := generated[i]; !ok {
				generated[i] = struct{}{}
				selections = append(selections, users[i])
				break
			}
		}
	}
	return selections
}

func shoveIntoMap(u []string, m map[string]struct{}) map[string]struct{} {
	for _, n := range u {
		m[n] = struct{}{}
	}
	return m
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, client *github.Client) ([]string, error) {
	logger := zerolog.Ctx(ctx)
	pendingLeafNodes := findLeafChildren(result)
	var requestedUsers []string
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	logger.Debug().Msgf("Collecting reviewers for %d pending leaf nodes", len(pendingLeafNodes))

	for _, child := range pendingLeafNodes {
		allUsers := make(map[string]struct{})
		allUsers = shoveIntoMap(child.RequestedUsers, allUsers)

		if len(child.RequestedTeams) > 0 {
			randomTeam := child.RequestedTeams[r.Intn(len(child.RequestedTeams))]
			teamInfo := strings.Split(randomTeam, "/")
			team, _, err := client.Teams.GetTeamBySlug(ctx, teamInfo[0], teamInfo[1])
			if err != nil {
				logger.Debug().Err(err).Msgf("Unable to get member listing for team %s", randomTeam)
				//return nil, errors.Wrapf(err, "Unable to get information for team %s", randomTeam)
			} else {
				teamMembers, err := listAllTeamMembers(ctx, client, team)
				if err != nil {
					logger.Debug().Err(err).Msgf("Unable to get member listing for team %s", randomTeam)
					//return nil, errors.Wrapf(err, "Unable to get member listing for team %s", randomTeam)
				} else {
					allUsers = shoveIntoMap(teamMembers, allUsers)
				}
			}
		}

		if len(child.RequestedOrganizations) > 0 {
			randomOrg := child.RequestedOrganizations[r.Intn(len(child.RequestedOrganizations))]
			orgMembers, err := listAllOrgMembers(ctx, client, randomOrg)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get member listing for org %s", randomOrg)
			}
			allUsers = shoveIntoMap(orgMembers, allUsers)
		}

		allCollaborators, err := listAllCollaborators(ctx, client, prctx.RepositoryOwner(), prctx.RepositoryName())
		if err != nil {
			return nil, errors.Wrap(err, "Unable to get collaborators")
		}
		collaboratorPermissions := make(map[string]string)

		if child.RequestedWriteCollaborators || child.RequestedAdmins {
			for _, c := range allCollaborators {
				perm, _, err := client.Repositories.GetPermissionLevel(ctx, prctx.RepositoryOwner(), prctx.RepositoryName(), c)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to determine permission level of %s on repo %s", c, prctx.RepositoryName())
				}
				collaboratorPermissions[c] = perm.GetPermission()
			}
		}

		if child.RequestedAdmins {
			var repoAdmins []string
			for _, c := range allCollaborators {
				if collaboratorPermissions[c] == "admin" {
					repoAdmins = append(repoAdmins, c)
				}
			}
			allUsers = shoveIntoMap(repoAdmins, allUsers)
		}

		if child.RequestedWriteCollaborators {
			var repoCollaborators []string
			for _, c := range allCollaborators {
				if collaboratorPermissions[c] == "write" {
					repoCollaborators = append(repoCollaborators, c)
				}
			}
			allUsers = shoveIntoMap(repoCollaborators, allUsers)
		}

		// Remove author before randomly selecting, since github will fail to assign _anyone_
		if _, ok := allUsers[prctx.Author()]; ok {
			delete(allUsers, prctx.Author())
		}

		// Remove any users who aren't collaborators, since github will fail to assign _anyone_
		for k := range allUsers {
			if collaboratorPerm, ok := collaboratorPermissions[k]; ok {
				if collaboratorPerm != "admin" && collaboratorPerm != "write" {
					delete(allUsers, k)
				}
			}
		}

		var allUserList []string
		for k := range allUsers {
			allUserList = append(allUserList, k)
		}

		logger.Debug().Msgf("Found %q total candidates for review after removing author; randomly selecting some", allUsers)
		randomSelection := selectRandomUsers(child.RequiredCount, allUserList)
		requestedUsers = append(requestedUsers, randomSelection...)
	}
	return requestedUsers, nil
}
