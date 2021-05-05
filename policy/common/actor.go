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

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
)

// Actors specifies who may take certain actions based on their username or
// team and organization memberships. The set of allowed actors is the union of
// all conditions in this structure.
type Actors struct {
	Users         []string `yaml:"users"`
	Teams         []string `yaml:"teams"`
	Organizations []string `yaml:"organizations"`

	// Deprecated: use Permissions with "admin" or "write"
	Admins             bool `yaml:"admins"`
	WriteCollaborators bool `yaml:"write_collaborators"`

	// A list of GitHub collaborator permissions that are allowed. Values may
	// be any of "admin", "maintain", "write", "triage", and "read".
	//
	// Each desired permission must be listed explicity. For example, even
	// though "admin" is a superset of "write" in GitHub, both "admin" and
	// "write" must be included in the list to allow users with either role.
	Permissions []pull.Permission
}

// IsEmpty returns true if no conditions for actors are defined.
func (a *Actors) IsEmpty() bool {
	return a == nil || (len(a.Users) == 0 && len(a.Teams) == 0 && len(a.Organizations) == 0 &&
		len(a.Permissions) == 0 && !a.Admins && !a.WriteCollaborators)
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

	perms := append([]pull.Permission(nil), a.Permissions...)
	if a.Admins {
		perms = append(perms, pull.PermissionAdmin)
	}
	if a.WriteCollaborators {
		perms = append(perms, pull.PermissionWrite)
	}

	userPerm, err := prctx.CollaboratorPermission(user)
	if err != nil {
		return false, err
	}
	if userPerm == pull.PermissionNone {
		return false, nil
	}

	for _, p := range perms {
		if userPerm == p {
			return true, nil
		}
	}
	return false, nil
}
