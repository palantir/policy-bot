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
	"time"
)

// MembershipContext defines methods to get information
// about about user membership in Github organizations and teams.
type MembershipContext interface {
	// IsTeamMember returns true if the user is a member of the given team.
	// Teams are specified as "org-name/team-name".
	IsTeamMember(team, user string) (bool, error)

	// IsOrgMember returns true if the user is a member of the given organzation.
	IsOrgMember(org, user string) (bool, error)
}

// Context is the context for a pull request. It defines methods to get
// information about the pull request and the VCS system containing the pull
// request (e.g. GitHub).
//
// A new Context should be created for each request, so implementations are not
// required to be thread-safe.
type Context interface {
	MembershipContext

	// Locator returns a locator string for the pull request. The locator
	// string is formated as "<owner>/<repository>#<number>"
	Locator() string

	// Author returns the username of the user who opened the pull request.
	Author() (string, error)

	// ChangedFiles returns the files that were changed in this pull request.
	ChangedFiles() ([]*File, error)

	// Commits returns the commits that are part of this pull request.
	Commits() ([]*Commit, error)

	// Comments lists all comments on a Pull Request
	Comments() ([]*Comment, error)

	// Reviews lists all reviews on a Pull Request
	Reviews() ([]*Review, error)

	// Branches returns the base (also known as target) and head branch names
	// of this pull request. Branches in this repository have no prefix, while
	// branches in forks are prefixed with the owner of the fork and a colon.
	// The base branch will always be unprefixed.
	Branches() (base string, head string, err error)
}

type FileStatus int

const (
	FileModified FileStatus = iota
	FileAdded
	FileDeleted
)

type File struct {
	Filename  string
	Status    FileStatus
	Additions int
	Deletions int
}

type Commit struct {
	// Order is the order it appears in the PR timeline.
	Order int

	SHA string

	// Author is the login name of the author. It is empty if the author is not
	// a real user.
	Author string

	// Commiter is the login name of the committer. It is empty if the
	// committer is not a real user.
	Committer string
}

// Users returns the login names of the users associated with this commit.
func (c *Commit) Users() []string {
	var users []string
	if c.Author != "" {
		users = append(users, c.Author)
	}
	if c.Committer != "" {
		users = append(users, c.Committer)
	}
	return users
}

type Comment struct {
	// Order is the order it appears in the PR timeline.
	Order int

	Author       string
	LastModified time.Time
	Body         string
}

type ReviewState string

const (
	ReviewApproved         ReviewState = "approved"
	ReviewChangesRequested             = "changes_requested"
	ReviewCommented                    = "commented"
	ReviewDismissed                    = "dismissed"
	ReviewPending                      = "pending"
)

type Review struct {
	// Order is the order it appears in the PR timeline.
	Order int

	Author       string
	LastModified time.Time
	State        ReviewState
	Body         string
}
