// Copyright 2026 Palantir Technologies, Inc.
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

package server

import (
	"testing"
	"time"

	"github.com/palantir/policy-bot/server/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigStatusDebounceWindow(t *testing.T) {
	t.Setenv("POLICYBOT_ENV_PREFIX", "TEST_POLICYBOT_")

	cfg, err := ParseConfig([]byte(`options:
  status_debounce_window: 3s
`))
	require.NoError(t, err)

	assert.Equal(t, 3*time.Second, cfg.Options.StatusDebounceWindow)
}

func TestParseConfigStatusDebounceWindowDefault(t *testing.T) {
	t.Setenv("POLICYBOT_ENV_PREFIX", "TEST_POLICYBOT_")

	cfg, err := ParseConfig([]byte(`options: {}`))
	require.NoError(t, err)

	assert.Equal(t, handler.DefaultDebounceWindow, cfg.Options.StatusDebounceWindow)
}
