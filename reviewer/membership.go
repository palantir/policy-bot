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

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

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
		Affiliation: "all",
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
