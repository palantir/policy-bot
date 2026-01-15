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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCodeowner(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType string
		expectedName string
	}{
		{
			name:         "user with @",
			input:        "@username",
			expectedType: "user",
			expectedName: "username",
		},
		{
			name:         "user without @",
			input:        "username",
			expectedType: "user",
			expectedName: "username",
		},
		{
			name:         "team with @",
			input:        "@org/team-name",
			expectedType: "team",
			expectedName: "org/team-name",
		},
		{
			name:         "team without @",
			input:        "org/team-name",
			expectedType: "team",
			expectedName: "org/team-name",
		},
		{
			name:         "team with nested path",
			input:        "@org/parent/child-team",
			expectedType: "team",
			expectedName: "org/parent/child-team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerType, name := ParseCodeowner(tt.input)
			assert.Equal(t, tt.expectedType, ownerType)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}

func TestCodeownersResultAllOwners(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		var result *CodeownersResult
		owners := result.AllOwners()
		assert.Nil(t, owners)
	})

	t.Run("empty owners", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{},
		}
		owners := result.AllOwners()
		assert.Empty(t, owners)
	})

	t.Run("single file single owner", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file.go": {"@user1"},
			},
		}
		owners := result.AllOwners()
		assert.Equal(t, []string{"@user1"}, owners)
	})

	t.Run("multiple files with overlapping owners", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file1.go": {"@user1", "@user2"},
				"file2.go": {"@user2", "@user3"},
				"file3.go": {"@org/team1"},
			},
		}
		owners := result.AllOwners()
		sort.Strings(owners) // Sort for deterministic comparison
		assert.Equal(t, []string{"@org/team1", "@user1", "@user2", "@user3"}, owners)
	})

	t.Run("deduplication", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file1.go": {"@user1", "@user1"},
				"file2.go": {"@user1"},
			},
		}
		owners := result.AllOwners()
		assert.Equal(t, []string{"@user1"}, owners)
	})
}

func TestCodeownersResultOwnershipGroups(t *testing.T) {
	t.Run("nil result", func(t *testing.T) {
		var result *CodeownersResult
		groups := result.OwnershipGroups()
		assert.Nil(t, groups)
	})

	t.Run("empty owners", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{},
		}
		groups := result.OwnershipGroups()
		assert.Nil(t, groups)
	})

	t.Run("single file single owner", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file.go": {"@team-a"},
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, "@team-a", groups[0].Key)
		assert.Equal(t, []string{"@team-a"}, groups[0].Owners)
		assert.Equal(t, []string{"file.go"}, groups[0].Files)
	})

	t.Run("multiple files same owner grouped together", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"a/file1.go": {"@team-a"},
				"a/file2.go": {"@team-a"},
				"a/file3.go": {"@team-a"},
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, "@team-a", groups[0].Key)
		assert.Equal(t, []string{"@team-a"}, groups[0].Owners)
		assert.Equal(t, []string{"a/file1.go", "a/file2.go", "a/file3.go"}, groups[0].Files)
	})

	t.Run("multiple distinct ownership groups", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"a/file.go": {"@team-a"},
				"b/file.go": {"@team-b"},
				"c/file.go": {"@team-c"},
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 3)

		// Groups should be sorted by key
		assert.Equal(t, "@team-a", groups[0].Key)
		assert.Equal(t, "@team-b", groups[1].Key)
		assert.Equal(t, "@team-c", groups[2].Key)
	})

	t.Run("owner order independent grouping", func(t *testing.T) {
		// Files with the same owners but listed in different order
		// should be grouped together
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file1.go": {"@user1", "@user2"},
				"file2.go": {"@user2", "@user1"},
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, "@user1,@user2", groups[0].Key)
		assert.Equal(t, []string{"@user1", "@user2"}, groups[0].Owners)
		assert.Equal(t, []string{"file1.go", "file2.go"}, groups[0].Files)
	})

	t.Run("mixed single and multiple owners", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"a/file.go":   {"@team-a"},
				"b/file.go":   {"@team-b", "@team-c"},
				"ab/file.go":  {"@team-a", "@team-b"},
				"ab/file2.go": {"@team-b", "@team-a"}, // Same as above, different order
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 3)

		// Find each group by key
		groupByKey := make(map[string]OwnershipGroup)
		for _, g := range groups {
			groupByKey[g.Key] = g
		}

		// Single owner group
		assert.Equal(t, []string{"a/file.go"}, groupByKey["@team-a"].Files)

		// Two owners group (@team-a, @team-b) - files should be grouped
		abGroup := groupByKey["@team-a,@team-b"]
		assert.Equal(t, []string{"@team-a", "@team-b"}, abGroup.Owners)
		assert.Equal(t, []string{"ab/file.go", "ab/file2.go"}, abGroup.Files)

		// Two owners group (@team-b, @team-c)
		bcGroup := groupByKey["@team-b,@team-c"]
		assert.Equal(t, []string{"@team-b", "@team-c"}, bcGroup.Owners)
		assert.Equal(t, []string{"b/file.go"}, bcGroup.Files)
	})

	t.Run("files with empty owners are skipped", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file1.go": {"@team-a"},
				"file2.go": {},
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, "@team-a", groups[0].Key)
	})

	t.Run("user and team owners mixed", func(t *testing.T) {
		result := &CodeownersResult{
			Owners: map[string][]string{
				"file1.go": {"@johndoe", "@org/team-a"},
				"file2.go": {"@org/team-a", "@johndoe"}, // Same owners, different order
			},
		}
		groups := result.OwnershipGroups()
		assert.Len(t, groups, 1)
		assert.Equal(t, "@johndoe,@org/team-a", groups[0].Key)
		assert.Equal(t, []string{"file1.go", "file2.go"}, groups[0].Files)
	})
}
