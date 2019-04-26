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

package pulltest

import (
	"github.com/palantir/policy-bot/pull"
)

type Context struct {
	OwnerValue  string
	RepoValue   string
	NumberValue int

	AuthorValue  string
	HeadSHAValue string

	BranchBaseName string
	BranchHeadName string

	ChangedFilesValue []*pull.File
	ChangedFilesError error

	CommitsValue []*pull.Commit
	CommitsError error

	CommentsValue []*pull.Comment
	CommentsError error

	ReviewsValue []*pull.Review
	ReviewsError error

	TeamMemberships     map[string][]string
	TeamMembershipError error

	OrgMemberships     map[string][]string
	OrgMembershipError error

	CollaboratorMemberships     map[string][]string
	CollaboratorMembershipError error
}

func (c *Context) RepositoryOwner() string {
	if c.OwnerValue != "" {
		return c.OwnerValue
	}
	return "pulltest"
}

func (c *Context) RepositoryName() string {
	if c.RepoValue != "" {
		return c.RepoValue
	}
	return "context"
}

func (c *Context) Number() int {
	if c.NumberValue > 0 {
		return c.NumberValue
	}
	return 1
}

func (c *Context) Author() string {
	return c.AuthorValue
}

func (c *Context) HeadSHA() string {
	return c.HeadSHAValue
}

func (c *Context) Branches() (base string, head string) {
	return c.BranchBaseName, c.BranchHeadName
}

func (c *Context) ChangedFiles() ([]*pull.File, error) {
	return c.ChangedFilesValue, c.ChangedFilesError
}

func (c *Context) Commits() ([]*pull.Commit, error) {
	return c.CommitsValue, c.CommitsError
}

func (c *Context) IsTeamMember(team, user string) (bool, error) {
	if c.TeamMembershipError != nil {
		return false, c.TeamMembershipError
	}

	for _, t := range c.TeamMemberships[user] {
		if t == team {
			return true, nil
		}
	}
	return false, nil
}

func (c *Context) IsOrgMember(org, user string) (bool, error) {
	if c.OrgMembershipError != nil {
		return false, c.OrgMembershipError
	}

	for _, o := range c.OrgMemberships[user] {
		if o == org {
			return true, nil
		}
	}
	return false, nil
}

func (c *Context) IsCollaborator(org, repo, user, desiredPerm string) (bool, error) {
	if c.CollaboratorMembershipError != nil {
		return false, c.CollaboratorMembershipError
	}

	for _, c := range c.CollaboratorMemberships[user] {
		if c == desiredPerm {
			return true, nil
		}
	}
	return false, nil
}

func (c *Context) Comments() ([]*pull.Comment, error) {
	return c.CommentsValue, c.CommentsError
}

func (c *Context) Reviews() ([]*pull.Review, error) {
	return c.ReviewsValue, c.ReviewsError
}

// assert that the test object implements the full interface
var _ pull.Context = &Context{}
