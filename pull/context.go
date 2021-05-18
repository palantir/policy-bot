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

	// TeamMembers returns the list of usernames in the given organization's team.
	TeamMembers(team string) ([]string, error)

	// OrganizationMembers returns the list of org member usernames in the given organization.
	OrganizationMembers(org string) ([]string, error)
}

// Context is the context for a pull request. It defines methods to get
// information about the pull request and the VCS system containing the pull
// request (e.g. GitHub).
//
// A new Context should be created for each request, so implementations are not
// required to be thread-safe.
type Context interface {
	MembershipContext

	// RepositoryOwner returns the owner of the repo that the pull request targets.
	RepositoryOwner() string

	// RepositoryName returns the repo that the pull request targets.
	RepositoryName() string

	// Number returns the number of the pull request.
	Number() int

	// Title returns the title of the pull request
	Title() string

	// Author returns the username of the user who opened the pull request.
	Author() string

	// CreatedAt returns the time when the pull request was created.
	CreatedAt() time.Time

	// IsOpen returns true when the state of the pull request is "open"
	IsOpen() bool

	// IsClosed returns true when the state of the pull request is "closed"
	IsClosed() bool

	// HeadSHA returns the SHA of the head commit of the pull request.
	HeadSHA() string

	// Branches returns the base (also known as target) and head branch names
	// of this pull request. Branches in this repository have no prefix, while
	// branches in forks are prefixed with the owner of the fork and a colon.
	// The base branch will always be unprefixed.
	Branches() (base string, head string)

	// ChangedFiles returns the files that were changed in this pull request.
	ChangedFiles() ([]*File, error)

	// Commits returns the commits that are part of this pull request. The
	// commit order is implementation dependent.
	Commits() ([]*Commit, error)

	// Comments lists all comments on a Pull Request. The comment order is
	// implementation dependent.
	Comments() ([]*Comment, error)

	// Reviews lists all reviews on a Pull Request. The review order is
	// implementation dependent.
	Reviews() ([]*Review, error)

	// IsDraft returns the draft status of the Pull Request.
	IsDraft() bool

	// RepositoryCollaborators returns the repository collaborators.
	RepositoryCollaborators() ([]*Collaborator, error)

	// CollaboratorPermission returns the permission level of user on the repository.
	CollaboratorPermission(user string) (Permission, error)

	// Teams lists the set of team collaborators, along with their respective
	// permission on a repo.
	Teams() (map[string]Permission, error)

	// RequestedReviewers returns any current and dismissed review requests on
	// the pull request.
	RequestedReviewers() ([]*Reviewer, error)

	// LatestStatuses returns a map of status check names to the latest result
	LatestStatuses() (map[string]string, error)

	// Labels returns a list of labels applied on the Pull Request
	Labels() ([]string, error)
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
	SHA             string
	Parents         []string
	CommittedViaWeb bool

	// Author is the login name of the author. It is empty if the author is not
	// a real user.
	Author string

	// Commiter is the login name of the committer. It is empty if the
	// committer is not a real user.
	Committer string

	// PushedAt is the timestamp when the commit was pushed. It is nil if that
	// information is not available for this commit.
	PushedAt *time.Time

	// Signature is the signature and details that was extracted from the commit.
	// It is nil if the commit has no signature
	Signature *Signature
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

type SignatureType string

const (
	SignatureGpg   SignatureType = "GpgSignature"
	SignatureSmime SignatureType = "SmimeSignature"
)

type Signature struct {
	Type    SignatureType
	IsValid bool
	KeyID   string
	Signer  string
	State   string
}

type Comment struct {
	CreatedAt time.Time
	Author    string
	Body      string
}

type ReviewState string

const (
	ReviewApproved         ReviewState = "approved"
	ReviewChangesRequested ReviewState = "changes_requested"
	ReviewCommented        ReviewState = "commented"
	ReviewDismissed        ReviewState = "dismissed"
	ReviewPending          ReviewState = "pending"
)

type Review struct {
	CreatedAt time.Time
	Author    string
	State     ReviewState
	Body      string
	SHA       string
}

type ReviewerType string

const (
	ReviewerUser ReviewerType = "user"
	ReviewerTeam ReviewerType = "team"
)

type Reviewer struct {
	Type    ReviewerType
	Name    string
	Removed bool
}

type Collaborator struct {
	Name        string
	Permissions []CollaboratorPermission
}

type CollaboratorPermission struct {
	Permission Permission

	// True if Permission is granted by a direct or team association with the
	// repository. If false, the permisssion is granted by the organization.
	ViaRepo bool
}
