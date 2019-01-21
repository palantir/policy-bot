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

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

type GitHubMembershipContext struct {
	ctx    context.Context
	client *github.Client

	teamIDs    map[string]int64
	membership map[string]bool
}

func NewGitHubMembershipContext(ctx context.Context, client *github.Client) *GitHubMembershipContext {
	return &GitHubMembershipContext{
		ctx:        ctx,
		client:     client,
		teamIDs:    make(map[string]int64),
		membership: make(map[string]bool),
	}
}

func membershipKey(group, user string) string {
	return group + ":" + user
}

func (mc *GitHubMembershipContext) IsTeamMember(team, user string) (bool, error) {
	key := membershipKey(team, user)
	org := strings.Split(team, "/")[0]

	id, ok := mc.teamIDs[team]
	if !ok {
		if err := mc.cacheTeamIDs(org); err != nil {
			return false, err
		}

		id, ok = mc.teamIDs[team]
		if !ok {
			return false, errors.Errorf("failed to get ID for team %s", team)
		}
	}

	isMember, ok := mc.membership[key]
	if ok {
		return isMember, nil
	}

	membership, _, err := mc.client.Teams.GetTeamMembership(mc.ctx, id, user)
	if err != nil && !isNotFound(err) {
		return false, errors.Wrap(err, "failed to get team membership")
	}

	isMember = membership != nil && membership.GetState() == "active"

	mc.membership[key] = isMember
	return isMember, nil
}

func (mc *GitHubMembershipContext) cacheTeamIDs(org string) error {
	var opt github.ListOptions
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

func (mc *GitHubMembershipContext) IsCollaborator(desiredPerm, org, repo, user string) (bool, error) {
	perm, _, err := mc.client.Repositories.GetPermissionLevel(mc.ctx, org, repo, user)
	if err != nil {
		return false, errors.Wrap(err, "failed to get repo admin permission")
	}

	return perm.GetPermission() == desiredPerm, nil
}
