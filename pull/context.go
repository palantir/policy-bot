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
	"sort"
	"strings"
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

	// TeamMembersWithDetails returns team members with full metadata (avatars, URLs).
	TeamMembersWithDetails(team string) ([]TeamMember, error)

	// TeamInfo returns team metadata for display purposes.
	TeamInfo(team string) (*TeamInfo, error)

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

	// EvaluationTimestamp returns the time at the start of the pull request
	// evaluation, usually the creation time of the context. All calls on the
	// same context should return the same value.
	EvaluationTimestamp() time.Time

	// RepositoryOwner returns the owner of the repo that the pull request targets.
	RepositoryOwner() string

	// RepositoryName returns the repo that the pull request targets.
	RepositoryName() string

	// RepositoryCustomProperties returns the custom properties of the repo that the pull request targets.
	// For an unset property, the key is _not_ present in the map.
	RepositoryCustomProperties() (map[string]CustomProperty, error)

	// Number returns the number of the pull request.
	Number() int

	// Title returns the title of the pull request
	Title() string

	// Body returns a struct that includes LastEditedAt for the pull request body
	Body() (*Body, error)

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

	// PushedAt returns the time at which the commit with sha was pushed. The
	// returned time may be after the actual push time, but must not be before.
	PushedAt(sha string) (time.Time, error)

	// Comments lists all comments on a Pull Request. The comment order is
	// implementation dependent.
	Comments() ([]*Comment, error)

	// Reviews lists all reviews on a Pull Request. The review order is
	// implementation dependent.
	Reviews() ([]*Review, error)

	// IsDraft returns the draft status of the Pull Request.
	IsDraft() bool

	// RepositoryCollaborators returns the repository collaborators.
	// Filters to collaborators with at least the specified permission level.
	// Filtering by permission can significantly improve performance.
	RepositoryCollaborators(minPermission Permission) ([]*Collaborator, error)

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

	// LatestWorkflowRuns returns the latest GitHub Actions workflow runs for
	// the pull request. The keys of the map are paths to the workflow files and
	// the values are the conclusions of the latest runs, one per event type.
	LatestWorkflowRuns() (map[string][]string, error)

	// Labels returns a list of labels applied on the Pull Request
	Labels() ([]string, error)

	// Codeowners returns the codeowners for files changed in this pull request.
	// Returns nil if no CODEOWNERS file exists in the repository.
	Codeowners() (*CodeownersResult, error)
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
	SignatureSSH   SignatureType = "SshSignature"
)

type Signature struct {
	Type           SignatureType
	IsValid        bool
	KeyID          string
	KeyFingerprint string
	Signer         string
	State          string
}

// Author represents a GitHub user who performed an action.
type Author struct {
	Login     string
	AvatarURL string
}

// NewAuthor creates an Author with the given login. Useful for tests.
func NewAuthor(login string) *Author {
	return &Author{Login: login}
}

type Comment struct {
	CreatedAt    time.Time
	LastEditedAt time.Time
	Author       *Author
	Body         string
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
	ID           string
	CreatedAt    time.Time
	LastEditedAt time.Time
	Author       *Author
	State        ReviewState
	Body         string
	SHA          string

	Teams []string
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
	// repository. If false, the permission is granted by the organization.
	ViaRepo bool
}

type Body struct {
	Body         string
	CreatedAt    time.Time
	Author       *Author
	LastEditedAt time.Time
}

type CustomProperty struct {
	String *string
	Array  []string
}

// CodeownersResult contains the owners for each changed file in a pull request.
type CodeownersResult struct {
	// Owners maps file paths to their owners. Owners are in the format
	// "@username" for users or "@org/team" for teams.
	Owners map[string][]string

	// OrphanFiles contains files that don't match any CODEOWNERS pattern.
	OrphanFiles []string
}

// HasOrphanFiles returns true if there are files without codeowners.
func (c *CodeownersResult) HasOrphanFiles() bool {
	return c != nil && len(c.OrphanFiles) > 0
}

// AllOwners returns a deduplicated list of all owners across all files.
func (c *CodeownersResult) AllOwners() []string {
	if c == nil {
		return nil
	}

	ownerSet := make(map[string]struct{})
	for _, fileOwners := range c.Owners {
		for _, owner := range fileOwners {
			ownerSet[owner] = struct{}{}
		}
	}

	owners := make([]string, 0, len(ownerSet))
	for owner := range ownerSet {
		owners = append(owners, owner)
	}
	return owners
}

// OwnersByOwner returns an inverted mapping of owner -> files.
// This is useful for displaying which files each owner is responsible for.
func (c *CodeownersResult) OwnersByOwner() map[string][]string {
	if c == nil {
		return nil
	}

	result := make(map[string][]string)
	for file, owners := range c.Owners {
		for _, owner := range owners {
			result[owner] = append(result[owner], file)
		}
	}
	return result
}

// ParseCodeowner parses a CODEOWNERS owner string and returns whether it's
// a user or team, along with the normalized name (without @ prefix).
// Examples:
//
//	"@username" -> ("user", "username")
//	"@org/team-name" -> ("team", "org/team-name")
func ParseCodeowner(owner string) (ownerType string, name string) {
	owner = strings.TrimPrefix(owner, "@")
	if strings.Contains(owner, "/") {
		return "team", owner
	}
	return "user", owner
}

// OwnershipGroup represents a unique set of owners for one or more files.
// Files with identical owner sets are grouped together.
type OwnershipGroup struct {
	// Key is a deterministic string representation of the sorted owner set,
	// used for grouping files with identical owners.
	Key string

	// Owners contains the owners in this group (e.g., "@team-a", "@user1").
	Owners []string

	// Files contains the file paths that belong to this ownership group.
	Files []string
}

// OwnershipGroups returns the unique ownership groups across all changed files.
// Files with identical owner sets are grouped together. The groups are returned
// in a deterministic order based on the sorted owner keys.
func (c *CodeownersResult) OwnershipGroups() []OwnershipGroup {
	if c == nil || len(c.Owners) == 0 {
		return nil
	}

	// Group files by their sorted owner set
	groupMap := make(map[string]*OwnershipGroup)

	for file, owners := range c.Owners {
		if len(owners) == 0 {
			continue
		}

		// Create a deterministic key by sorting owners
		sortedOwners := make([]string, len(owners))
		copy(sortedOwners, owners)
		sort.Strings(sortedOwners)
		key := strings.Join(sortedOwners, ",")

		if group, exists := groupMap[key]; exists {
			group.Files = append(group.Files, file)
		} else {
			groupMap[key] = &OwnershipGroup{
				Key:    key,
				Owners: sortedOwners,
				Files:  []string{file},
			}
		}
	}

	// Convert map to slice and sort for deterministic output
	groups := make([]OwnershipGroup, 0, len(groupMap))
	for _, group := range groupMap {
		// Sort files within each group for deterministic output
		sort.Strings(group.Files)
		groups = append(groups, *group)
	}

	// Sort groups by key for deterministic output
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Key < groups[j].Key
	})

	return groups
}
