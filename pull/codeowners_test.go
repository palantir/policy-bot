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
