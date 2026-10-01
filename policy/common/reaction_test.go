// Copyright 2025 Palantir Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestReactionContentUnmarshal(t *testing.T) {
	t.Run("acceptsEveryGitHubContentValue", func(t *testing.T) {
		var m Methods
		err := yaml.Unmarshal([]byte(`
reactions:
  - "+1"
  - "-1"
  - laugh
  - confused
  - heart
  - hooray
  - rocket
  - eyes
`), &m)
		require.NoError(t, err)
		require.Len(t, m.Reactions, 8)
		assert.Equal(t, ReactionContent("+1"), m.Reactions[0])
		assert.Equal(t, ReactionContent("eyes"), m.Reactions[7])
	})

	t.Run("rejectsUnknownContent", func(t *testing.T) {
		var m Methods
		err := yaml.Unmarshal([]byte("reactions: [\"thumbsup\"]"), &m)
		require.Error(t, err, "an unknown reaction should not parse")
		assert.Contains(t, err.Error(), `invalid reaction "thumbsup"`)
	})

	t.Run("rejectsEmojiCodes", func(t *testing.T) {
		// ":+1:" and "👍" are the spellings used by the "comments" method, so
		// they are the likely mistake here; the error should be clear.
		for _, content := range []string{":+1:", "👍"} {
			var m Methods
			err := yaml.Unmarshal([]byte("reactions: [\""+content+"\"]"), &m)
			require.Errorf(t, err, "%q should not parse as a reaction", content)
			assert.Contains(t, err.Error(), "not emoji or emoji codes")
		}
	})
}

func TestReactionMatches(t *testing.T) {
	m := &Methods{Reactions: []ReactionContent{"+1", "rocket"}}

	assert.True(t, m.ReactionMatches("+1"))
	assert.True(t, m.ReactionMatches("rocket"))
	assert.False(t, m.ReactionMatches("-1"))
	assert.False(t, m.ReactionMatches(""))

	empty := &Methods{}
	assert.False(t, empty.ReactionMatches("+1"), "no configured reactions matches nothing")
}

func TestGetReactionsDefaults(t *testing.T) {
	defaults := &Methods{Reactions: []ReactionContent{"+1"}}

	t.Run("inheritsFromDefaults", func(t *testing.T) {
		m := &Methods{Defaults: defaults}
		assert.Equal(t, []ReactionContent{"+1"}, m.GetReactions())
	})

	t.Run("ruleOverridesDefaults", func(t *testing.T) {
		m := &Methods{Reactions: []ReactionContent{"rocket"}, Defaults: defaults}
		assert.Equal(t, []ReactionContent{"rocket"}, m.GetReactions())
	})

	t.Run("emptyListDisablesDefaults", func(t *testing.T) {
		m := &Methods{Reactions: []ReactionContent{}, Defaults: defaults}
		assert.Empty(t, m.GetReactions(), "an explicit empty list turns the method off")
	})
}
