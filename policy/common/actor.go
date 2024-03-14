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

package common

import (
	"context"
	"sort"

	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// Actors specifies who may take certain actions based on their username or
// team and organization memberships. The set of allowed actors is the union of
// all conditions in this structure.
type Actors struct {
	Users         []string `yaml:"users" json:"users"`
	Teams         []string `yaml:"teams" json:"teams"`
	Organizations []string `yaml:"organizations" json:"organizations"`

	// Deprecated: use Permissions with "admin" or "write"
	Admins             bool `yaml:"admins" json:"-"`
	WriteCollaborators bool `yaml:"write_collaborators" json:"-"`

	// A list of GitHub collaborator permissions that are allowed. Values may
	// be any of "admin", "maintain", "write", "triage", and "read".
	Permissions []pull.Permission `yaml:"permissions" json:"permissions"`
}

// IsEmpty returns true if no conditions for actors are defined.
func (a *Actors) IsEmpty() bool {
	return a == nil || (len(a.Users) == 0 && len(a.Teams) == 0 && len(a.Organizations) == 0 &&
		len(a.Permissions) == 0 && !a.Admins && !a.WriteCollaborators)
}

// GetPermissions returns unique permissions ordered from most to least
// permissive. It includes the permissions from the deprecated Admins and
// WriteCollaborators fields.
func (a *Actors) GetPermissions() []pull.Permission {
	permSet := make(map[pull.Permission]struct{})
	for _, p := range a.Permissions {
		permSet[p] = struct{}{}
	}
	if a.Admins {
		permSet[pull.PermissionAdmin] = struct{}{}
	}
	if a.WriteCollaborators {
		permSet[pull.PermissionWrite] = struct{}{}
	}

	perms := make([]pull.Permission, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}

	// sort by decreasing privilege
	sort.Slice(perms, func(i, j int) bool { return perms[i] > perms[j] })
	return perms
}

// IsActor returns true if the given user satisfies at least one of the
// conditions in this structure.
func (a *Actors) IsActor(ctx context.Context, prctx pull.Context, user string) (bool, error) {
	for _, u := range a.Users {
		if user == u {
			return true, nil
		}
	}

	for _, t := range a.Teams {
		member, err := prctx.IsTeamMember(t, user)
		if err != nil {
			return false, errors.Wrap(err, "failed to get team membership")
		}
		if member {
			return true, nil
		}
	}

	for _, o := range a.Organizations {
		member, err := prctx.IsOrgMember(o, user)
		if err != nil {
			return false, errors.Wrap(err, "failed to get org membership")
		}
		if member {
			return true, nil
		}
	}

	userPerm, err := prctx.CollaboratorPermission(user)
	if err != nil {
		return false, err
	}
	if userPerm == pull.PermissionNone {
		return false, nil
	}

	for _, p := range a.GetPermissions() {
		if userPerm >= p {
			return true, nil
		}
	}
	return false, nil
}
