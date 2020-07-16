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

package pull

import (
	"context"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

type GitHubMembershipContext struct {
	ctx    context.Context
	client *github.Client

	teamIDs     map[string]int64
	membership  map[string]bool
	orgMembers  map[string][]string
	teamMembers map[string][]string
}

func NewGitHubMembershipContext(ctx context.Context, client *github.Client) *GitHubMembershipContext {
	return &GitHubMembershipContext{
		ctx:         ctx,
		client:      client,
		teamIDs:     make(map[string]int64),
		membership:  make(map[string]bool),
		orgMembers:  make(map[string][]string),
		teamMembers: make(map[string][]string),
	}
}

func membershipKey(group, user string) string {
	return group + ":" + user
}

func (mc *GitHubMembershipContext) IsTeamMember(team, user string) (bool, error) {
	key := membershipKey(team, user)

	id, err := mc.getTeamID(team)
	if err != nil {
		return false, errors.Errorf("failed to get ID for team %s", team)
	}

	isMember, ok := mc.membership[key]
	if ok {
		return isMember, nil
	}

	membership, _, err := GetTeamMembership(mc.ctx, mc.client, id, user)
	if err != nil && !isNotFound(err) {
		return false, errors.Wrap(err, "failed to get team membership")
	}

	isMember = membership != nil && membership.GetState() == "active"

	mc.membership[key] = isMember
	return isMember, nil
}

func (mc *GitHubMembershipContext) getTeamID(team string) (int64, error) {
	org := strings.Split(team, "/")[0]
	id, ok := mc.teamIDs[team]
	if !ok {
		if err := mc.cacheTeamIDs(org); err != nil {
			return 0, err
		}

		id, ok = mc.teamIDs[team]
		if !ok {
			return 0, errors.Errorf("failed to get ID for team %s", team)
		}
	}
	return id, nil
}

func (mc *GitHubMembershipContext) cacheTeamIDs(org string) error {
	opt := github.ListOptions{
		PerPage: 100,
	}

	for {
		teams, res, err := mc.client.Teams.ListTeams(mc.ctx, org, &opt)
		if err != nil {
			return errors.Wrap(err, "failed to list organization teams")
		}

		for _, t := range teams {
			key := org + "/" + t.GetSlug()
			mc.teamIDs[key] = t.GetID()
		}

		if res.NextPage == 0 {
			break
		}
		opt.Page = res.NextPage
	}
	return nil
}

func (mc *GitHubMembershipContext) IsOrgMember(org, user string) (bool, error) {
	key := membershipKey(org, user)

	isMember, ok := mc.membership[key]
	if ok {
		return isMember, nil
	}

	isMember, _, err := mc.client.Organizations.IsMember(mc.ctx, org, user)
	if err != nil {
		return false, errors.Wrap(err, "failed to get organization membership")
	}

	mc.membership[key] = isMember
	return isMember, nil
}

func (mc *GitHubMembershipContext) IsCollaborator(org, repo, user, desiredPerm string) (bool, error) {
	perm, _, err := mc.client.Repositories.GetPermissionLevel(mc.ctx, org, repo, user)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get repo %s permission", desiredPerm)
	}

	return perm.GetPermission() == desiredPerm, nil
}

func (mc *GitHubMembershipContext) OrganizationMembers(org string) ([]string, error) {
	members, ok := mc.orgMembers[org]
	if !ok {
		opt := &github.ListMembersOptions{
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}

		for {
			users, resp, err := mc.client.Organizations.ListMembers(mc.ctx, org, opt)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to list members of org %s page %d", org, opt.Page)
			}
			for _, u := range users {
				members = append(members, u.GetLogin())
				// And cache these values for later lookups
				mc.membership[membershipKey(org, u.GetLogin())] = true
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		mc.orgMembers[org] = members
	}
	return members, nil
}

func (mc *GitHubMembershipContext) TeamMembers(team string) ([]string, error) {
	members, ok := mc.teamMembers[team]
	if !ok {
		opt := &github.TeamListTeamMembersOptions{
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}

		teamID, err := mc.getTeamID(team)
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to get information for team %s", team)
		}

		for {
			users, resp, err := ListTeamMembers(mc.ctx, mc.client, teamID, opt)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to list team %s members page %d", team, opt.Page)
			}
			for _, u := range users {
				members = append(members, u.GetLogin())
				// And cache these values for later lookups
				mc.membership[membershipKey(team, u.GetLogin())] = true
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		mc.teamMembers[team] = members
	}
	return members, nil
}
