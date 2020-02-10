// Copyright 2020 Palantir Technologies, Inc.
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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestRegexpUnmarshal(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var r Regexp
		require.NoError(t, json.Unmarshal([]byte(`"test/.*"`), &r), "failed to unmarshal json")

		assert.True(t, r.Matches("test/path/to/file.txt"))
		assert.False(t, r.Matches("something/else/path/to/file.txt"))
	})

	t.Run("jsonEmpty", func(t *testing.T) {
		var r Regexp
		require.NoError(t, json.Unmarshal([]byte(`""`), &r), "failed to unmarshal json")

		assert.True(t, r.Matches("any string"))
		assert.True(t, r.Matches(""))
	})

	t.Run("jsonError", func(t *testing.T) {
		var r Regexp
		require.Error(t, json.Unmarshal([]byte(`"this(is an unclosed [group"`), &r), "invalid regexp unmarshalled without error")
	})

	t.Run("yaml", func(t *testing.T) {
		var r Regexp
		require.NoError(t, yaml.Unmarshal([]byte(`"test/.*"`), &r), "failed to unmarshal yaml")

		assert.True(t, r.Matches("test/path/to/file.txt"))
		assert.False(t, r.Matches("something/else/path/to/file.txt"))
	})

	t.Run("yamlEmpty", func(t *testing.T) {
		var r Regexp
		require.NoError(t, yaml.Unmarshal([]byte(`""`), &r), "failed to unmarshal yaml")

		assert.True(t, r.Matches("any string"))
		assert.True(t, r.Matches(""))
	})

	t.Run("yamlError", func(t *testing.T) {
		var r Regexp
		require.Error(t, yaml.Unmarshal([]byte(`"this(is an unclosed [group"`), &r), "invalid regexp unmarshalled without error")
	})
}
