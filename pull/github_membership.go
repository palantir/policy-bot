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

	"github.com/google/go-github/v59/github"
	"github.com/pkg/errors"
)

type GitHubMembershipContext struct {
	ctx    context.Context
	client *github.Client

	membership  map[string]bool
	orgMembers  map[string][]string
	teamMembers map[string][]string
}

func NewGitHubMembershipContext(ctx context.Context, client *github.Client) *GitHubMembershipContext {
	return &GitHubMembershipContext{
		ctx:         ctx,
		client:      client,
		membership:  make(map[string]bool),
		orgMembers:  make(map[string][]string),
		teamMembers: make(map[string][]string),
	}
}

func membershipKey(group, user string) string {
	return group + ":" + user
}

func splitTeam(team string) (org, slug string, err error) {
	parts := strings.Split(team, "/")
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid team format: %s", team)
	}
	return parts[0], parts[1], nil
}

func (mc *GitHubMembershipContext) IsTeamMember(team, user string) (bool, error) {
	key := membershipKey(team, user)

	org, slug, err := splitTeam(team)
	if err != nil {
		return false, err
	}

	isMember, ok := mc.membership[key]
	if ok {
		return isMember, nil
	}

	membership, _, err := mc.client.Teams.GetTeamMembershipBySlug(mc.ctx, org, slug, user)
	if err != nil && !isNotFound(err) {
		return false, errors.Wrap(err, "failed to get team membership")
	}

	isMember = membership != nil && membership.GetState() == "active"
	mc.membership[key] = isMember

	return isMember, nil
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

		org, slug, err := splitTeam(team)
		if err != nil {
			return nil, err
		}

		for {
			users, resp, err := mc.client.Teams.ListTeamMembersBySlug(mc.ctx, org, slug, opt)
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
