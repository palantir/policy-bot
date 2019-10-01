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

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

func findLeafChildren(result common.Result) []common.Result {
	var r []common.Result
	if len(result.Children) == 0 {
		if result.Status == common.StatusPending && result.Error != nil {
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

func listAllTeamMembers(ctx context.Context, client *github.Client, team *github.Team) ([]*github.User, error) {
	opt := &github.TeamListTeamMembersOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	// get all pages of results
	var allUsers []*github.User

	for {
		repos, resp, err := client.Teams.ListTeamMembers(ctx, team.GetID(), opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list team %s members page %d", team.GetName(), opt.Page)
		}

		allUsers = append(allUsers, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

func FindRandomRequesters(ctx context.Context, prctx pull.Context, result common.Result, client *github.Client) ([]string, error) {
	logger := zerolog.Ctx(ctx)
	// look for requested reviewers
	pendingLeafNodes := findLeafChildren(result)
	var requestedUsers []string
	for _, child := range pendingLeafNodes {

		// TODO(asvoboda): additional cases here for admin, write collaborator and organization selections
		selectionType := rand.Intn(2)

		var randomAssignee string
		switch selectionType {
		case 0:
			// pick a user
			randomAssignee = child.RequestedUsers[rand.Intn(len(child.RequestedUsers))]
			logger.Debug().Str("user", randomAssignee).Msg("Select from set of hardcoded usernames")
		case 1:
			//pick a user from a team
			randomTeam := child.RequestedTeams[rand.Intn(len(child.RequestedTeams))]
			teamInformation := strings.Split(randomTeam, "/")
			team, _, err := client.Teams.GetTeamBySlug(ctx, teamInformation[0], teamInformation[1])
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get information for team %q", randomTeam)
			}

			teamMembers, err := listAllTeamMembers(ctx, client, team)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to get member listing for team %q", randomTeam)
			}

			randomAssignee = teamMembers[rand.Intn(len(teamMembers))].GetLogin()
			logger.Debug().Str("user", randomAssignee).Str("team", randomTeam).Msg("Select from set of teams")
		}
		requestedUsers = append(requestedUsers, randomAssignee)
	}

	// remove PR author if they exist in the possible list, since they cannot be assigned to their own PRs
	for i, u := range requestedUsers {
		if u == prctx.Author() {
			requestedUsers = append(requestedUsers[:i], requestedUsers[i+1:]...)
		}
	}

	return requestedUsers, nil
}
