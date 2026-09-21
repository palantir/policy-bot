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

package predicate

import (
	"context"
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestCommitMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("allModeSubjectMatches", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^JIRA-[0-9]+:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "JIRA-123: Add feature"},
				{SHA: "def456", MessageHeadline: "JIRA-456: Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "all commits should match pattern")
			assert.Contains(t, result.Description, "All 2 commits")
		}
	})

	t.Run("allModeOneDoesNotMatch", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^JIRA-[0-9]+:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "JIRA-123: Add feature"},
				{SHA: "def456", MessageHeadline: "Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "one commit does not match")
			assert.Contains(t, result.Description, "def456")
		}
	})

	t.Run("anyModeOneMatches", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "any",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^JIRA-[0-9]+:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "Fix typo"},
				{SHA: "def456", MessageHeadline: "JIRA-456: Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "at least one commit matches")
			assert.Contains(t, result.Description, "def456")
		}
	})

	t.Run("anyModeNoneMatch", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "any",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^JIRA-[0-9]+:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "Fix typo"},
				{SHA: "def456", MessageHeadline: "Update docs"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "no commits match")
			assert.Contains(t, result.Description, "No commits match")
		}
	})

	t.Run("notMatchesPattern", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			NotMatches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("WIP|fixup")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "Add feature"},
				{SHA: "def456", MessageHeadline: "Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "no commits contain WIP or fixup")
		}
	})

	t.Run("notMatchesFails", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			NotMatches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("WIP|fixup")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "Add feature"},
				{SHA: "def456", MessageHeadline: "WIP: Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "one commit contains WIP")
		}
	})

	t.Run("matchesAndNotMatches", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^[A-Z]+")),
			},
			NotMatches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("WIP")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "FEAT: Add feature"},
				{SHA: "def456", MessageHeadline: "FIX: Fix bug"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "all start with uppercase and none have WIP")
		}
	})

	t.Run("bodyScope", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "any",
			Scope: "body",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("(?i)breaking change")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{
					SHA:             "abc123",
					MessageHeadline: "Update API",
					MessageBody:     "This is a breaking change to the API",
				},
				{
					SHA:             "def456",
					MessageHeadline: "Fix bug",
					MessageBody:     "Minor bug fix",
				},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "one commit body mentions breaking change")
		}
	})

	t.Run("fullScope", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "any",
			Scope: "full",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("important")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{
					SHA:             "abc123",
					MessageHeadline: "Update feature",
					MessageBody:     "This is an important update",
				},
				{
					SHA:             "def456",
					MessageHeadline: "Important fix",
					MessageBody:     "",
				},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "commits contain 'important' in subject or body")
		}
	})

	t.Run("defaultMode", func(t *testing.T) {
		p := &CommitMessages{
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^feat:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "feat: add feature"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "default mode should be 'all'")
		}
	})

	t.Run("defaultScope", func(t *testing.T) {
		p := &CommitMessages{
			Mode: "all",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile("^feat:")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "feat: add feature", MessageBody: "details"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "default scope should be 'subject'")
		}
	})

	t.Run("noCommits", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
			Matches: []common.Regexp{
				common.NewCompiledRegexp(regexp.MustCompile(".*")),
			},
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.False(t, result.Satisfied, "no commits should fail")
			assert.Contains(t, result.Description, "No commits found")
		}
	})

	t.Run("invalidMode", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "invalid",
			Scope: "subject",
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "test"},
			},
		}

		_, err := p.Evaluate(ctx, prctx)
		assert.Error(t, err, "invalid mode should return error")
		assert.Contains(t, err.Error(), "invalid mode")
	})

	t.Run("invalidScope", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "invalid",
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "test"},
			},
		}

		_, err := p.Evaluate(ctx, prctx)
		assert.Error(t, err, "invalid scope should return error")
		assert.Contains(t, err.Error(), "invalid scope")
	})

	t.Run("noPatterns", func(t *testing.T) {
		p := &CommitMessages{
			Mode:  "all",
			Scope: "subject",
		}

		prctx := &pulltest.Context{
			CommitsValue: []*pull.Commit{
				{SHA: "abc123", MessageHeadline: "any message"},
			},
		}

		result, err := p.Evaluate(ctx, prctx)
		if assert.NoError(t, err) {
			assert.True(t, result.Satisfied, "no patterns should always pass")
		}
	})
}

func TestCommitMessagesTrigger(t *testing.T) {
	p := &CommitMessages{
		Mode:  "all",
		Scope: "subject",
	}

	assert.Equal(t, common.TriggerCommit, p.Trigger())
}
