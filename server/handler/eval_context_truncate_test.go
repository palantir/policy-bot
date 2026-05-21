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
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestTruncateStatusDescription exercises the helper directly. It guards two
// invariants: the output never exceeds GitHub's 140-rune cap, and when
// truncation occurs the caller is pointed at the details URL via the suffix.
func TestTruncateStatusDescription(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantEqual string // when non-empty, output must match exactly
		wantTrunc bool   // when true, output must end with the truncation suffix
	}{
		{
			name:      "empty string passes through",
			input:     "",
			wantEqual: "",
		},
		{
			name:      "short string passes through",
			input:     "All checks passed",
			wantEqual: "All checks passed",
		},
		{
			name:      "exact-limit string passes through",
			input:     strings.Repeat("a", gitHubStatusDescriptionMaxLen),
			wantEqual: strings.Repeat("a", gitHubStatusDescriptionMaxLen),
		},
		{
			name:      "one-over-limit string gets truncated",
			input:     strings.Repeat("a", gitHubStatusDescriptionMaxLen+1),
			wantTrunc: true,
		},
		{
			name: "long has_status description gets truncated and suffixed",
			input: "0/1 rules approved. required ci checks: 0/1 required conditions met. " +
				"status checks: Waiting on: pre-commit-check, sonar-component-1, " +
				"sonar-component-2; Failing: Tests with Coverage",
			wantTrunc: true,
		},
		{
			name:      "trailing punctuation is trimmed before suffix",
			input:     strings.Repeat("a", gitHubStatusDescriptionMaxLen-10) + ", , , , ,, more",
			wantTrunc: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateStatusDescription(tc.input)
			if tc.wantEqual != "" || tc.input == "" {
				assert.Equal(t, tc.wantEqual, got, "expected unchanged passthrough")
				return
			}
			assert.LessOrEqual(t, utf8.RuneCountInString(got), gitHubStatusDescriptionMaxLen,
				"truncated output must fit GitHub's %d-rune description cap (got %d runes): %q",
				gitHubStatusDescriptionMaxLen, utf8.RuneCountInString(got), got)
			if tc.wantTrunc {
				assert.True(t, strings.HasSuffix(got, statusDescriptionTruncationSuffix),
					"truncated output must end with %q so the reader follows the details link; got %q",
					statusDescriptionTruncationSuffix, got)
				assert.False(t, strings.HasSuffix(strings.TrimSuffix(got, statusDescriptionTruncationSuffix), ","),
					"trailing punctuation should be trimmed before the suffix; got %q", got)
			}
		})
	}
}

// TestTruncateStatusDescription_HandlesMultiByteRunes ensures the helper
// counts runes (not bytes) so we don't slice in the middle of a multi-byte
// character and emit invalid UTF-8 to GitHub.
func TestTruncateStatusDescription_HandlesMultiByteRunes(t *testing.T) {
	// Each "—" is 3 bytes but 1 rune. 200 of them is 200 runes / 600 bytes,
	// well over the 140-rune limit so truncation must engage.
	input := strings.Repeat("—", 200)

	got := truncateStatusDescription(input)
	assert.LessOrEqual(t, utf8.RuneCountInString(got), gitHubStatusDescriptionMaxLen,
		"rune count must respect the cap even for multi-byte input")
	assert.True(t, utf8.ValidString(got), "output must be valid UTF-8")
}

// TestEvalContext_PostStatusTruncatesLongDescription drives the EvalContext
// path the webhook handler uses and verifies that an over-limit has_status
// description is truncated before reaching the captured status. The policy
// references enough required checks that the rendered description exceeds
// 140 runes, exercising the truncation branch end-to-end through
// EvaluatePolicy → PostStatus.
func TestEvalContext_PostStatusTruncatesLongDescription(t *testing.T) {
	ctx := context.Background()

	// Many required statuses → long "Waiting on: ..." segment that will
	// blow past 140 runes once combined with the parent rule prefix.
	const policyYAML = `
approval_rules:
  - name: required ci checks for sonarcloud and friends
    requires:
      conditions:
        has_status:
          statuses:
            - "sonar-component-aaaaaaaaaaaaaaaa"
            - "sonar-component-bbbbbbbbbbbbbbbb"
            - "sonar-component-cccccccccccccccc"
            - "sonar-component-dddddddddddddddd"
            - "sonar-component-eeeeeeeeeeeeeeee"
            - "sonar-component-ffffffffffffffff"
policy:
  approval:
    - "required ci checks for sonarcloud and friends"
`

	var cfg policy.Config
	require.NoError(t, yaml.Unmarshal([]byte(policyYAML), &cfg))
	evaluator, err := policy.ParsePolicy(&cfg, nil)
	require.NoError(t, err)

	prctx := &pulltest.Context{
		OwnerValue:          "testorg",
		RepoValue:           "testrepo",
		NumberValue:         42,
		StateValue:          "open",
		HeadSHAValue:        "abc123",
		BranchBaseName:      "main",
		BranchHeadName:      "feature",
		LatestStatusesValue: map[string]string{
			// all six required checks missing → "Waiting on: ..." with every
			// long status name listed
		},
	}

	ec := &EvalContext{
		Options: &PullEvaluationOptions{
			StatusCheckContext: "policy-bot",
		},
		PublicURL:      "https://policy-bot.example.com",
		PullContext:    prctx,
		SkipPostStatus: true,
	}

	_, err = ec.EvaluatePolicy(ctx, evaluator)
	require.NoError(t, err)
	require.NotNil(t, ec.Status, "EvaluatePolicy should set ec.Status when SkipPostStatus is true")

	desc := ec.Status.GetDescription()
	assert.LessOrEqual(t, utf8.RuneCountInString(desc), gitHubStatusDescriptionMaxLen,
		"posted description must respect GitHub's 140-rune cap; got %d runes: %q",
		utf8.RuneCountInString(desc), desc)
	assert.True(t, strings.HasSuffix(desc, statusDescriptionTruncationSuffix),
		"long descriptions must end with %q so reviewers know to click the details link; got %q",
		statusDescriptionTruncationSuffix, desc)
	// TargetURL must still be populated so the truncation suffix actually
	// points somewhere actionable.
	assert.NotEmpty(t, ec.Status.GetTargetURL(),
		"TargetURL must be set so the truncation suffix refers to a real details page")
}
