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
			{Name: "mhaypenny", Permission: pull.PermissionAdmin},
			{Name: "jstrawnickel", Permission: pull.PermissionWrite},
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

func TestIsEmpty(t *testing.T) {
	a := &Actors{}
	assert.True(t, a.IsEmpty(), "Actors struct was not empty")

	a = &Actors{Users: []string{"user"}}
	assert.False(t, a.IsEmpty(), "Actors struct was empty")

	a = &Actors{Teams: []string{"org/team"}}
	assert.False(t, a.IsEmpty(), "Actors struct was empty")

	a = &Actors{Organizations: []string{"org"}}
	assert.False(t, a.IsEmpty(), "Actors struct was empty")

	a = nil
	assert.True(t, a.IsEmpty(), "nil struct was not empty")
}
