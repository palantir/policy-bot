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
	"testing"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsActor(t *testing.T) {
	ctx := context.Background()
	prctx := &pulltest.Context{
		TeamMemberships: map[string][]string{
			"mhaypenny": {"cool-org/team1", "regular-org/team2"},
		},
		OrgMemberships: map[string][]string{
			"mhaypenny": {"cool-org", "regular-org"},
		},
		CollaboratorsValue: []*pull.Collaborator{
			{
				Name: "mhaypenny",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionAdmin},
				},
			},
			{
				Name: "jstrawnickel",
				Permissions: []pull.CollaboratorPermission{
					{Permission: pull.PermissionWrite},
				},
			},
		},
	}

	assertActor := func(t *testing.T, a *Actors, user string) {
		isActor, err := a.IsActor(ctx, prctx, user)
		require.NoError(t, err)
		assert.Truef(t, isActor, "%s is not an actor", user)
	}

	assertNotActor := func(t *testing.T, a *Actors, user string) {
		isActor, err := a.IsActor(ctx, prctx, user)
		require.NoError(t, err)
		assert.Falsef(t, isActor, "%s is an actor", user)
	}

	t.Run("users", func(t *testing.T) {
		a := &Actors{
			Users: []string{"mhaypenny"},
		}

		assertActor(t, a, "mhaypenny")
		assertNotActor(t, a, "ttest")
	})

	t.Run("teams", func(t *testing.T) {
		a := &Actors{
			Teams: []string{"regular-org/team2"},
		}

		assertActor(t, a, "mhaypenny")
		assertNotActor(t, a, "ttest")
	})

	t.Run("organizations", func(t *testing.T) {
		a := &Actors{
			Organizations: []string{"cool-org"},
		}

		assertActor(t, a, "mhaypenny")
		assertNotActor(t, a, "ttest")
	})

	t.Run("admins", func(t *testing.T) {
		a := &Actors{Admins: true}

		assertActor(t, a, "mhaypenny")
		assertNotActor(t, a, "jstrawnickel")
		assertNotActor(t, a, "ttest")
	})

	t.Run("write", func(t *testing.T) {
		a := &Actors{WriteCollaborators: true}

		assertActor(t, a, "jstrawnickel")
		assertActor(t, a, "mhaypenny")
		assertNotActor(t, a, "ttest")
	})

	t.Run("permissions", func(t *testing.T) {
		a := &Actors{
			Permissions: []pull.Permission{pull.PermissionTriage},
		}

		assertActor(t, a, "mhaypenny")
		assertActor(t, a, "jstrawnickel")
		assertNotActor(t, a, "ttest")
	})
}

func TestIsActorCodeowners(t *testing.T) {
	ctx := context.Background()

	t.Run("user codeowner", func(t *testing.T) {
		prctx := &pulltest.Context{
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"src/main.go": {"@mhaypenny", "@other-user"},
				},
			},
		}

		a := &Actors{Codeowners: true}

		isActor, err := a.IsActor(ctx, prctx, "mhaypenny")
		require.NoError(t, err)
		assert.True(t, isActor, "codeowner should be an actor")

		isActor, err = a.IsActor(ctx, prctx, "other-user")
		require.NoError(t, err)
		assert.True(t, isActor, "codeowner should be an actor")

		isActor, err = a.IsActor(ctx, prctx, "random-user")
		require.NoError(t, err)
		assert.False(t, isActor, "non-codeowner should not be an actor")
	})

	t.Run("team codeowner", func(t *testing.T) {
		prctx := &pulltest.Context{
			TeamMemberships: map[string][]string{
				"team-member": {"myorg/my-team"},
			},
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"src/main.go": {"@myorg/my-team"},
				},
			},
		}

		a := &Actors{Codeowners: true}

		isActor, err := a.IsActor(ctx, prctx, "team-member")
		require.NoError(t, err)
		assert.True(t, isActor, "team member should be an actor")

		isActor, err = a.IsActor(ctx, prctx, "non-member")
		require.NoError(t, err)
		assert.False(t, isActor, "non-team-member should not be an actor")
	})

	t.Run("no codeowners file", func(t *testing.T) {
		prctx := &pulltest.Context{
			CodeownersValue: nil,
		}

		a := &Actors{Codeowners: true}

		isActor, err := a.IsActor(ctx, prctx, "anyone")
		require.NoError(t, err)
		assert.False(t, isActor, "should not be an actor when no CODEOWNERS file")
	})

	t.Run("empty owners for file", func(t *testing.T) {
		prctx := &pulltest.Context{
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{},
			},
		}

		a := &Actors{Codeowners: true}

		isActor, err := a.IsActor(ctx, prctx, "anyone")
		require.NoError(t, err)
		assert.False(t, isActor, "should not be an actor when no owners defined")
	})

	t.Run("codeowners disabled", func(t *testing.T) {
		prctx := &pulltest.Context{
			CodeownersValue: &pull.CodeownersResult{
				Owners: map[string][]string{
					"src/main.go": {"@mhaypenny"},
				},
			},
		}

		a := &Actors{Codeowners: false}

		isActor, err := a.IsActor(ctx, prctx, "mhaypenny")
		require.NoError(t, err)
		assert.False(t, isActor, "should not be an actor when codeowners is disabled")
	})
}

func TestIsEmpty(t *testing.T) {
	a := &Actors{}
	assert.True(t, a.IsZero(), "Actors struct was not empty")

	a = &Actors{Users: []string{"user"}}
	assert.False(t, a.IsZero(), "Actors struct was empty")

	a = &Actors{Teams: []string{"org/team"}}
	assert.False(t, a.IsZero(), "Actors struct was empty")

	a = &Actors{Organizations: []string{"org"}}
	assert.False(t, a.IsZero(), "Actors struct was empty")

	a = &Actors{Codeowners: true}
	assert.False(t, a.IsZero(), "Actors struct was empty")

	a = nil
	assert.True(t, a.IsZero(), "nil struct was not empty")
}
