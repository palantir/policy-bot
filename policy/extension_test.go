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

package policy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestApprovalExtendsValidation(t *testing.T) {
	for _, test := range []struct {
		name, input, message string
	}{
		{"both approval keys", `policy: {approval: [review], approval_extends: {remote: example/policies, path: .policy.yml}}`, "approval and approval_extends cannot both be specified"},
		{"empty approval", `policy: {approval: [], approval_extends: {remote: example/policies, path: .policy.yml}}`, "approval and approval_extends cannot both be specified"},
		{"empty extension", `policy: {approval_extends: {}}`, "invalid approval_extends"},
		{"missing repository", `policy: {approval_extends: {remote: example/}}`, "invalid approval_extends"},
		{"missing path", `policy: {approval_extends: {remote: example/policies}}`, "approval_extends requires a path"},
		{"unresolved extension", `policy: {approval_extends: {remote: example/policies, ref: main, path: config/policy.yml}}`, "approval_extends requires a resolved parent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := extensionConfig(t, test.input)
			parsed, err := ParsePolicy(c, nil)
			require.ErrorContains(t, err, test.message)
			assert.Nil(t, parsed)
		})
	}
}

func TestApprovalExtensionNullValues(t *testing.T) {
	parent := extensionConfig(t, `
policy: {approval: [review]}
approval_rules: [{name: review, requires: {count: 1}}]
`)
	child := extensionConfig(t, `policy: {approval: null, approval_extends: {remote: example/policies, path: .policy.yml}}`)
	parsed, err := resolveExtension(child, parent, nil)
	require.NoError(t, err)
	assert.Equal(t, common.StatusPending, parsed.Evaluate(context.Background(), &pulltest.Context{}).Status)

	standalone := extensionConfig(t, `
policy: {approval: [review], approval_extends: null}
approval_rules: [{name: review, requires: {count: 1}}]
`)
	eval, err := ParsePolicy(standalone, nil)
	require.NoError(t, err)
	assert.Equal(t, common.StatusPending, eval.Evaluate(context.Background(), &pulltest.Context{}).Status)
}

func TestApprovalExtensionRuleScope(t *testing.T) {
	parent := extensionConfig(t, `
policy:
  approval: [review, {slot: {name: extra, required: true}}, {slot: {name: empty}}]
approval_rules:
  - name: review
    requires: {count: 1, users: [platform]}
`)
	child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies, path: .policy.yml}}
slots: {extra: [review]}
approval_rules:
  - name: review
    requires: {count: 1, users: [service]}
`)
	file, err := Parse(parent, nil)
	require.NoError(t, err, "an incomplete file must parse before it can be extended")
	standalone, err := file.Resolve()
	require.ErrorContains(t, err, "required slots are empty")
	assert.Nil(t, standalone)
	parsed, err := resolveExtension(child, parent, nil)
	require.NoError(t, err)
	for _, tt := range []struct {
		users  []string
		status common.EvaluationStatus
	}{
		{[]string{"service"}, common.StatusPending},
		{[]string{"platform"}, common.StatusPending},
		{[]string{"platform", "service"}, common.StatusApproved},
	} {
		t.Run(fmt.Sprint(tt.users), func(t *testing.T) {
			prctx := &pulltest.Context{}
			for _, user := range tt.users {
				prctx.ReviewsValue = append(prctx.ReviewsValue, &pull.Review{Author: user, State: pull.ReviewApproved})
			}
			result := parsed.Evaluate(context.Background(), prctx)
			require.NoError(t, result.Error)
			assert.Equal(t, tt.status, result.Status)
		})
	}
	assert.Equal(t, parent.ApprovalRules[0], parsed.FindApprovalRule("", "review"))
	assert.Equal(t, child.ApprovalRules[0], parsed.FindApprovalRule("extra", "review"))
	assert.Nil(t, parsed.FindApprovalRule("empty", "review"))
	assert.Nil(t, parsed.FindApprovalRule("missing", "review"))
	assert.Nil(t, parsed.FindApprovalRule("extra", "unknown"))
}

func TestApprovalExtensionContentReplacement(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		required      bool
		comments      []string
		wantError     bool
	}{
		{"required default", "", true, []string{"parent"}, false},
		{"overridden", "slots: {extra: [review]}", true, []string{"child"}, false},
		{"cleared", "slots: {extra: []}", false, nil, false},
		{"required cleared", "slots: {extra: []}", true, nil, true},
		{"null", "slots: {extra: null}", false, nil, false},
		{"required null", "slots: {extra: null}", true, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parent := extensionConfig(t, fmt.Sprintf(`
policy: {approval: [{slot: {name: extra, required: %t}}]}
slots: {extra: [review]}
approval_defaults: {options: {methods: {comments: [parent]}}}
approval_rules: [{name: review, requires: {count: 1}}]
`, tt.required))
			child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies, path: .policy.yml}}
approval_defaults: {options: {methods: {comments: [child]}}}
approval_rules: [{name: review, requires: {count: 1}}]
`+tt.content)
			parsed, err := resolveExtension(child, parent, nil)
			if tt.wantError {
				require.ErrorContains(t, err, "required slots are empty")
				return
			}
			require.NoError(t, err)
			rule := parsed.FindApprovalRule("extra", "review")
			if len(tt.comments) == 0 {
				assert.Nil(t, rule)
			} else {
				require.NotNil(t, rule)
				assert.Equal(t, tt.comments, rule.Options.GetMethods().GetComments())
			}
		})
	}
}

func TestApprovalExtensionMethods(t *testing.T) {
	parent := extensionConfig(t, `
policy: {approval: [retained, {slot: {name: extra}}]}
slots: {extra: [retained, replaced]}
approval_rules:
  - name: retained
    options: {methods: {comments: [parent], github_review: false}}
    requires: {count: 1}
  - name: replaced
    if: {has_labels: [security]}
    options: {methods: {comments: [replaced], github_review: true}}
    requires: {count: 1}
  - name: unused
    options: {methods: {comments: [parent-unused]}}
`)
	child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies, path: .policy.yml}}
slots: {extra: [{or: [review]}], unused: [unused]}
approval_rules:
  - name: review
    if: {has_labels: [service]}
    options: {methods: {comments: [child], github_review: false}}
    requires: {count: 1}
  - name: unused
    options: {methods: {comments: [child-unused]}}
`)
	parsed, err := resolveExtension(child, parent, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"parent", "replaced", "parent-unused", "child", "child-unused"}, extensionComments(parsed))
	assert.Equal(t, common.TriggerCommit|common.TriggerComment|common.TriggerLabel, parsed.Trigger())
	assert.Nil(t, parsed.FindApprovalRule("extra", "replaced"))
	assert.Nil(t, parsed.FindApprovalRule("extra", "retained"))
	assert.Nil(t, parsed.FindApprovalRule("unused", "unused"))
	rule := parsed.FindApprovalRule("extra", "review")
	require.NotNil(t, rule)
	assert.Equal(t, []string{"child"}, rule.Options.GetMethods().GetComments())

	t.Run("standalone includes unused rules", func(t *testing.T) {
		file, err := Parse(parent, nil)
		require.NoError(t, err)
		standalone, err := file.Resolve()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"parent", "replaced", "parent-unused"}, extensionComments(standalone))
	})
}

func TestApprovalExtensionInvalidSlotContents(t *testing.T) {
	for _, test := range []struct {
		name, slot string
		parent     bool
	}{
		{"replaced parent content", "extra", true},
		{"unused child content", "unused", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := extensionConfig(t, "policy: {approval: [{slot: {name: extra}}]}")
			child := extensionConfig(t, "policy: {approval_extends: {remote: example/policies, path: .policy.yml}}\nslots: {extra: []}")
			badContent := Slots{test.slot: {"foreign"}}
			if test.parent {
				parent.Slots = badContent
				child.ApprovalRules = []*approval.Rule{{Name: "foreign"}}
			} else {
				child.Slots = badContent
				parent.ApprovalRules = []*approval.Rule{{Name: "foreign"}}
			}
			_, err := resolveExtension(child, parent, nil)
			require.ErrorContains(t, err, "undefined rule 'foreign'")
		})
	}
}

func TestApprovalExtensionDisapproval(t *testing.T) {
	parent := extensionConfig(t, `
policy:
  approval: [review]
  disapproval:
    requires: {users: [parent]}
    options: {methods: {disapprove: {comments: [parent-block]}}}
approval_rules: [{name: review}]
`)
	child := extensionConfig(t, `
policy:
  approval_extends: {remote: example/policies, path: .policy.yml}
  disapproval:
    requires: {users: [child]}
    options:
      methods:
        disapprove: {comments: [child-block]}
        revoke: {comments: [child-clear]}
`)
	parsed, err := resolveExtension(child, parent, nil)
	require.NoError(t, err)
	comments := extensionComments(parsed)
	assert.NotContains(t, comments, "parent-block")
	assert.Contains(t, comments, "child-block")
	assert.Contains(t, comments, "child-clear")
	parentBlock := &pull.Comment{Author: "parent", Body: "parent-block"}
	childBlock := &pull.Comment{Author: "child", Body: "child-block", CreatedAt: time.Unix(1, 0)}
	childRevoke := &pull.Comment{Author: "child", Body: "child-clear", CreatedAt: time.Unix(2, 0)}
	for _, test := range []struct {
		name     string
		comments []*pull.Comment
		status   common.EvaluationStatus
	}{
		{"parent disapproval ignored", []*pull.Comment{parentBlock}, common.StatusApproved},
		{"child disapproval", []*pull.Comment{parentBlock, childBlock}, common.StatusDisapproved},
		{"child revocation", []*pull.Comment{parentBlock, childBlock, childRevoke}, common.StatusApproved},
	} {
		t.Run(test.name, func(t *testing.T) {
			prctx := &pulltest.Context{CommentsValue: test.comments}
			result := parsed.Evaluate(context.Background(), prctx)
			require.NoError(t, result.Error)
			assert.Equal(t, test.status, result.Status)
		})
	}

	t.Run("no child disapproval", func(t *testing.T) {
		child.Policy.Disapproval = nil
		parsed, err := resolveExtension(child, parent, nil)
		require.NoError(t, err)
		prctx := &pulltest.Context{CommentsValue: []*pull.Comment{parentBlock}}
		result := parsed.Evaluate(context.Background(), prctx)
		require.NoError(t, result.Error)
		assert.Equal(t, common.StatusApproved, result.Status)
		assert.NotContains(t, extensionComments(parsed), "parent-block")
	})
}

func TestApprovalExtensionSharedDefaults(t *testing.T) {
	opts := &GlobalOptions{
		IgnoreEditedComments: new(true),
		ApprovalDefaults: &approval.Defaults{Options: &approval.Options{
			AllowAuthor: new(true),
			Methods:     &common.Methods{Comments: []string{"server"}},
		}},
	}
	parent := extensionConfig(t, `
policy: {approval: [review, {slot: {name: extra}}]}
approval_defaults: {options: {methods: {comments: [parent]}}}
approval_rules: [{name: review, options: {ignore_edited_comments: false}}]
`)
	child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies, path: .policy.yml}}
slots: {extra: [review]}
approval_rules: [{name: review, options: {ignore_edited_comments: false}}]
`)
	parsed, err := resolveExtension(child, parent, opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"parent", "server"}, extensionComments(parsed))
	for _, slot := range []string{"", "extra"} {
		rule := parsed.FindApprovalRule(slot, "review")
		assert.True(t, rule.Options.IsAllowAuthor())
		assert.True(t, rule.Options.IsIgnoreEditedComments())
	}
}

func extensionConfig(t *testing.T, input string) *Config {
	t.Helper()
	var c Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(input), &c))
	return &c
}

func extensionComments(parsed *ParsedPolicy) []string {
	var comments []string
	for _, methods := range parsed.Methods() {
		comments = append(comments, methods.GetComments()...)
	}
	return comments
}

func resolveExtension(child, parent *Config, opts *GlobalOptions) (*ParsedPolicy, error) {
	extension, err := Parse(child, opts)
	if err != nil {
		return nil, err
	}
	base, err := Parse(parent, opts)
	if err != nil {
		return nil, err
	}
	return extension.Extend(base).Resolve()
}
