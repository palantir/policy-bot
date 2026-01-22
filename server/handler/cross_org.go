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

package handler

import (
	"context"
	"strings"
	"sync"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CrossOrgMembershipContext struct {
	ctx           context.Context
	lookupClient  *github.Client
	installations githubapp.InstallationsService
	clientCreator githubapp.ClientCreator
	globalCache   pull.GlobalCache

	mbrCtxs sync.Map // map[string]pull.MembershipContext
}

func NewCrossOrgMembershipContext(ctx context.Context, client *github.Client, orgName string, installations githubapp.InstallationsService, clientCreator githubapp.ClientCreator, globalCache pull.GlobalCache) *CrossOrgMembershipContext {
	mbrCtx := &CrossOrgMembershipContext{
		ctx:           ctx,
		lookupClient:  client,
		installations: installations,
		clientCreator: clientCreator,
		globalCache:   globalCache,
	}
	mbrCtx.mbrCtxs.Store(orgName, pull.NewGitHubMembershipContext(ctx, client, globalCache))
	return mbrCtx
}

func (c *CrossOrgMembershipContext) getCtxForTeam(team string) (pull.MembershipContext, error) {
	return c.getCtxForOrg(strings.Split(team, "/")[0])
}

func (c *CrossOrgMembershipContext) getCtxForOrg(name string) (pull.MembershipContext, error) {
	if val, ok := c.mbrCtxs.Load(name); ok {
		return val.(pull.MembershipContext), nil
	}

	// Create a new membership context for this org
	mbrCtx, err := c.createMembershipContext(name)
	if err != nil {
		return nil, err
	}

	// Use LoadOrStore to handle concurrent creation - if another goroutine
	// already stored a context, use that one instead
	actual, _ := c.mbrCtxs.LoadOrStore(name, mbrCtx)
	return actual.(pull.MembershipContext), nil
}

func (c *CrossOrgMembershipContext) createMembershipContext(name string) (pull.MembershipContext, error) {
	org, _, err := c.lookupClient.Organizations.Get(c.ctx, name)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	installation, err := c.installations.GetByOwner(c.ctx, org.GetLogin())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup installation ID for org '%s' or there is no such installation", name)
	}

	client, err := c.clientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		return nil, err
	}

	return pull.NewGitHubMembershipContext(c.ctx, client, c.globalCache), nil
}

func (c *CrossOrgMembershipContext) IsTeamMember(team, user string) (bool, error) {
	mbrCtx, err := c.getCtxForTeam(team)
	if err != nil {
		return false, err
	}
	return mbrCtx.IsTeamMember(team, user)
}

func (c *CrossOrgMembershipContext) IsOrgMember(org, user string) (bool, error) {
	mbrCtx, err := c.getCtxForOrg(org)
	if err != nil {
		return false, err
	}
	return mbrCtx.IsOrgMember(org, user)
}

func (c *CrossOrgMembershipContext) OrganizationMembers(org string) ([]string, error) {
	mbrCtx, err := c.getCtxForOrg(org)
	if err != nil {
		return nil, err
	}
	return mbrCtx.OrganizationMembers(org)
}

func (c *CrossOrgMembershipContext) TeamMembers(team string) ([]string, error) {
	mbrCtx, err := c.getCtxForTeam(team)
	if err != nil {
		return nil, err
	}
	return mbrCtx.TeamMembers(team)
}

func (c *CrossOrgMembershipContext) TeamMembersWithDetails(team string) ([]pull.TeamMember, error) {
	mbrCtx, err := c.getCtxForTeam(team)
	if err != nil {
		return nil, err
	}
	return mbrCtx.TeamMembersWithDetails(team)
}

func (c *CrossOrgMembershipContext) TeamInfo(team string) (*pull.TeamInfo, error) {
	mbrCtx, err := c.getCtxForTeam(team)
	if err != nil {
		return nil, err
	}
	return mbrCtx.TeamInfo(team)
}
