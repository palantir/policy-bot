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
	"sort"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

const (
	LogKeyLeafNode = "leaf_node"
)

type Selection struct {
	Users []string
	Teams []string
}

// Difference returns a new Selection with the users and teams that must be
// added to the pull request given the reviewers that already exist. Reviewers
// that were explicitly removed are not added again.
func (s Selection) Difference(reviewers []*pull.Reviewer) Selection {
	users := make(map[string]bool)
	teams := make(map[string]bool)
	for _, r := range reviewers {
		switch r.Type {
		case pull.ReviewerUser:
			users[r.Name] = true
		case pull.ReviewerTeam:
			teams[r.Name] = true
		}
	}

	var newUsers []string
	for _, u := range s.Users {
		if !users[u] {
			newUsers = append(newUsers, u)
		}
	}

	var newTeams []string
	for _, t := range s.Teams {
		if !teams[t] {
			newTeams = append(newTeams, t)
		}
	}

	return Selection{
		Users: newUsers,
		Teams: newTeams,
	}
}

// IsEmpty returns true if the Selection has no users or teams.
func (s Selection) IsEmpty() bool {
	return len(s.Users) == 0 && len(s.Teams) == 0
}

// FindRequests returns all pending leaf results with review requests enabled.
func FindRequests(result *common.Result) []*common.Result {
	if result.Status != common.StatusPending {
		return nil
	}

	var reqs []*common.Result
	for _, c := range result.Children {
		reqs = append(reqs, FindRequests(c)...)
	}
	if len(result.Children) == 0 && result.ReviewRequestRule != nil && result.Error == nil {
		reqs = append(reqs, result)
	}
	return reqs
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

func getPossibleReviewers(prctx pull.Context, users map[string]struct{}, collaborators []*pull.Collaborator) []string {
	var possibleReviewers []string
	for _, c := range collaborators {
		_, exists := users[c.Name]
		if c.Name != prctx.Author() && exists {
			possibleReviewers = append(possibleReviewers, c.Name)
		}
	}

	// We need reviewer selection to be consistent when using a fixed random
	// seed, so sort the reviewers before returning.
	sort.Strings(possibleReviewers)
	return possibleReviewers
}

func SelectReviewers(ctx context.Context, prctx pull.Context, results []*common.Result, r *rand.Rand) (Selection, error) {
	selection := Selection{}

	for _, child := range results {
		logger := zerolog.Ctx(ctx).With().Str(LogKeyLeafNode, child.Name).Logger()
		childCtx := logger.WithContext(ctx)

		switch child.ReviewRequestRule.Mode {
		case common.RequestModeTeams:
			if err := selectTeamReviewers(childCtx, prctx, &selection, child); err != nil {
				return selection, err
			}
		case common.RequestModeAllUsers, common.RequestModeRandomUsers:
			if err := selectUserReviewers(childCtx, prctx, &selection, child, r); err != nil {
				return selection, err
			}
		default:
			return selection, fmt.Errorf("unknown reviewer selection mode: %s", child.ReviewRequestRule.Mode)
		}
	}
	return selection, nil
}

func selectTeamReviewers(ctx context.Context, prctx pull.Context, selection *Selection, result *common.Result) error {
	logger := zerolog.Ctx(ctx)

	eligibleTeams, err := prctx.Teams()
	if err != nil {
		return err
	}

	var teams []string
	for team, perm := range eligibleTeams {
		switch {
		case requestsTeam(result, prctx.RepositoryOwner()+"/"+team):
			teams = append(teams, team)
		case requestsPermission(result, perm):
			teams = append(teams, team)
		}
	}

	logger.Debug().Msgf("Requesting %d teams for review", len(teams))
	selection.Teams = append(selection.Teams, teams...)
	return nil
}

func selectUserReviewers(ctx context.Context, prctx pull.Context, selection *Selection, result *common.Result, r *rand.Rand) error {
	logger := zerolog.Ctx(ctx)

	allUsers := make(map[string]struct{})
	for _, user := range result.ReviewRequestRule.Users {
		allUsers[user] = struct{}{}
	}

	if len(result.ReviewRequestRule.Teams) > 0 {
		logger.Debug().Msg("Selecting from teams for review")
		teamsToUsers, err := selectTeamMembers(prctx, result.ReviewRequestRule.Teams)
		if err != nil {
			logger.Warn().Err(err).Msgf("failed to get member listing for teams, skipping team member selection")
		}
		for _, users := range teamsToUsers {
			for _, user := range users {
				allUsers[user] = struct{}{}
			}
		}
	}

	if len(result.ReviewRequestRule.Organizations) > 0 {
		logger.Debug().Msg("Selecting from organizations for review")
		orgMembers, err := selectOrgMembers(prctx, result.ReviewRequestRule.Organizations)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to get member listing for org, skipping org member selection")
		}
		for _, user := range orgMembers {
			allUsers[user] = struct{}{}
		}
	}

	collaborators, err := prctx.RepositoryCollaborators()
	if err != nil {
		return errors.Wrap(err, "failed to list repository collaborators")
	}

	if len(result.ReviewRequestRule.Permissions) > 0 {
		logger.Debug().Msg("Selecting from collaborators by permission for review")
		for _, c := range collaborators {
			if c.PermissionViaRepo && requestsPermission(result, c.Permission) {
				allUsers[c.Name] = struct{}{}
			}
		}
	}

	possibleReviewers := getPossibleReviewers(prctx, allUsers, collaborators)
	if len(possibleReviewers) == 0 {
		logger.Debug().Msg("Found 0 eligible reviewers; skipping review request")
		return nil
	}

	switch result.ReviewRequestRule.Mode {
	case common.RequestModeAllUsers:
		logger.Debug().Msgf("Found %d eligible reviewers; selecting all", len(possibleReviewers))
		selection.Users = append(selection.Users, possibleReviewers...)

	case common.RequestModeRandomUsers:
		count := result.ReviewRequestRule.RequiredCount
		selectedUsers := selectRandomUsers(count, possibleReviewers, r)

		logger.Debug().Msgf("Found %d eligible reviewers; randomly selecting %d", len(possibleReviewers), count)
		selection.Users = append(selection.Users, selectedUsers...)
	}
	return nil
}

func requestsTeam(r *common.Result, team string) bool {
	for _, t := range r.ReviewRequestRule.Teams {
		if t == team {
			return true
		}
	}
	return false
}

func requestsPermission(r *common.Result, perm pull.Permission) bool {
	for _, p := range r.ReviewRequestRule.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
