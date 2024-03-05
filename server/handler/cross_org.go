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

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CrossOrgMembershipContext struct {
	ctx           context.Context
	lookupClient  *github.Client
	installations githubapp.InstallationsService
	clientCreator githubapp.ClientCreator

	mbrCtxs map[string]pull.MembershipContext
}

func NewCrossOrgMembershipContext(ctx context.Context, client *github.Client, orgName string, installations githubapp.InstallationsService, clientCreator githubapp.ClientCreator) *CrossOrgMembershipContext {
	mbrCtx := &CrossOrgMembershipContext{
		ctx:           ctx,
		lookupClient:  client,
		installations: installations,
		clientCreator: clientCreator,
		mbrCtxs:       make(map[string]pull.MembershipContext),
	}
	mbrCtx.mbrCtxs[orgName] = pull.NewGitHubMembershipContext(ctx, client)
	return mbrCtx
}

func (c *CrossOrgMembershipContext) getCtxForOrg(name string) (pull.MembershipContext, error) {
	mbrCtx, ok := c.mbrCtxs[name]
	if !ok {
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

		mbrCtx = pull.NewGitHubMembershipContext(c.ctx, client)
		c.mbrCtxs[name] = mbrCtx
	}

	return mbrCtx, nil
}

func (c *CrossOrgMembershipContext) IsTeamMember(team, user string) (bool, error) {
	org := strings.Split(team, "/")[0]
	mbrCtx, err := c.getCtxForOrg(org)
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
	org := strings.Split(team, "/")[0]
	mbrCtx, err := c.getCtxForOrg(org)
	if err != nil {
		return nil, err
	}
	return mbrCtx.TeamMembers(team)
}
