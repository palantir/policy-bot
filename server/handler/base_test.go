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

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCheckRunOptions(t *testing.T) {
	tests := map[string]struct {
		state              string
		expectedStatus     string
		expectedConclusion string
		hasCompletedAt     bool
	}{
		"pending_maps_to_in_progress": {
			state:              "pending",
			expectedStatus:     "in_progress",
			expectedConclusion: "",
			hasCompletedAt:     false,
		},
		"success_maps_to_completed_success": {
			state:              "success",
			expectedStatus:     "completed",
			expectedConclusion: "success",
			hasCompletedAt:     true,
		},
		"failure_maps_to_completed_failure": {
			state:              "failure",
			expectedStatus:     "completed",
			expectedConclusion: "failure",
			hasCompletedAt:     true,
		},
		"error_maps_to_completed_action_required": {
			state:              "error",
			expectedStatus:     "completed",
			expectedConclusion: "action_required",
			hasCompletedAt:     true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			detailsURL := "https://example.com/details"
			opts := NewCheckRunOptions("policy-bot: main", "abc123", test.state, "test message", &detailsURL)

			assert.Equal(t, "policy-bot: main", opts.Name)
			assert.Equal(t, "abc123", opts.HeadSHA)
			assert.Equal(t, test.expectedStatus, opts.GetStatus())
			assert.Equal(t, test.expectedConclusion, opts.GetConclusion())

			assert.NotNil(t, opts.StartedAt, "StartedAt should always be set")

			if test.hasCompletedAt {
				assert.NotNil(t, opts.CompletedAt, "CompletedAt should be set for completed states")
			} else {
				assert.Nil(t, opts.CompletedAt, "CompletedAt should not be set for in_progress")
			}

			require.NotNil(t, opts.Output)
			assert.Equal(t, "test message", opts.Output.GetTitle())
			assert.Equal(t, "test message", opts.Output.GetSummary())
			assert.Equal(t, "https://example.com/details", opts.GetDetailsURL())
		})
	}
}

func TestNewCheckRunOptions_NilDetailsURL(t *testing.T) {
	opts := NewCheckRunOptions("policy-bot: main", "abc123", "success", "installed", nil)

	assert.Nil(t, opts.DetailsURL)
	assert.Equal(t, "policy-bot: main", opts.Name)
	assert.Equal(t, "completed", opts.GetStatus())
	assert.Equal(t, "success", opts.GetConclusion())
}
