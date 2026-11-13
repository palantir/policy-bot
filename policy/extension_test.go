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

func TestApprovalExtendsDecoding(t *testing.T) {
	invalid := map[string]string{
		"both approval keys":        `policy: {approval: [review], approval_extends: {remote: example/policies}}`,
		"empty approval":            `policy: {approval: [], approval_extends: {remote: example/policies}}`,
		"null approval":             `policy: {approval: null, approval_extends: {remote: example/policies}}`,
		"null extension":            `policy: {approval_extends: null}`,
		"empty extension":           `policy: {approval_extends: {}}`,
		"scalar extension":          `policy: {approval_extends: example/policies}`,
		"list extension":            `policy: {approval_extends: []}`,
		"unknown extension key":     `policy: {approval_extends: {remote: example/policies, other: true}}`,
		"unknown policy key":        `policy: {approval_extends: {remote: example/policies}, other: true}`,
		"duplicate null extension":  `policy: {approval_extends: null, approval_extends: {remote: example/policies}}`,
		"duplicate approval":        `policy: {approval: null, approval: []}`,
		"duplicate remote":          `policy: {approval_extends: {remote: example/policies, remote: example/other}}`,
		"missing repository":        `policy: {approval_extends: {remote: example/}}`,
		"merged approval":           `policy: {<<: {approval: null}, approval_extends: {remote: example/policies}}`,
		"merged extension":          `policy: {<<: {approval_extends: {remote: example/policies}}, approval: []}`,
		"merged null extension":     `policy: {<<: {approval_extends: null}}`,
		"aliased null":              `policy: {approval: &empty null, approval_extends: *empty}`,
		"root merged null approval": "<<: {policy: {approval: null}}\npolicy: {approval_extends: {remote: example/policies}}",
		"root merged extension":     "<<: {policy: {approval_extends: {remote: example/policies}}}\npolicy: {approval: null}",
	}
	for name, input := range invalid {
		t.Run(name, func(t *testing.T) {
			var c Config
			assert.Error(t, yaml.UnmarshalStrict([]byte(input), &c))
		})
	}
	for _, input := range []string{
		`policy: {approval_extends: {remote: example/policies, ref: main, path: config/policy.yml}}`,
		`policy: {<<: {approval_extends: {remote: example/policies}}}`,
		`policy: {approval_extends: {<<: {remote: example/default}, remote: example/policies}}`,
		`policy: &extension {approval_extends: {remote: example/policies}}
<<: {policy: *extension}`,
	} {
		var c Config
		require.NoError(t, yaml.UnmarshalStrict([]byte(input), &c))
		_, err := ParsePolicy(&c, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authenticated simulation")
	}
}

func TestPolicyMergePreservesApproval(t *testing.T) {
	c := extensionConfig(t, `
<<: {policy: {approval: [review]}}
policy: {disapproval: {}}
approval_rules: [{name: review, requires: {count: 1}}]
`)
	parsed, err := ParsePolicy(c, nil)
	require.NoError(t, err)
	result := parsed.Evaluate(context.Background(), &pulltest.Context{})
	require.NoError(t, result.Error)
	assert.Equal(t, common.StatusPending, result.Status)

	c = extensionConfig(t, `
policy: {<<: {approval: [missing]}, approval: []}
`)
	parsed, err = ParsePolicy(c, nil)
	require.NoError(t, err)
	assert.Equal(t, common.StatusApproved, parsed.Evaluate(context.Background(), &pulltest.Context{}).Status)
}

func TestApprovalExtensionRuleScope(t *testing.T) {
	parent := extensionConfig(t, `
policy:
  approval: [review, {slot: {name: extra, required: true}}]
approval_rules:
  - name: review
    requires: {count: 1, users: [platform]}
`)
	child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies}}
slots: {extra: [review]}
approval_rules:
  - name: review
    requires: {count: 1, users: [service]}
`)
	_, err := ParsePolicy(parent, nil)
	require.ErrorContains(t, err, "required slots are empty")
	parsed, err := ParsePolicyWithParent(child, parent, nil)
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
			require.NotNil(t, findSlotResult(&result, "extra"))
		})
	}
	assert.Same(t, parent.ApprovalRules[0], parsed.FindApprovalRule("", "review"))
	assert.Same(t, child.ApprovalRules[0], parsed.FindApprovalRule("extra", "review"))
	assert.Nil(t, parsed.FindApprovalRule("missing", "review"))
}

func TestApprovalExtensionContentReplacement(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		required      bool
		comments      []string
		wantError     bool
	}{
		{"omitted", "", false, []string{"parent"}, false},
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
policy: {approval_extends: {remote: example/policies}}
approval_defaults: {options: {methods: {comments: [child]}}}
approval_rules: [{name: review, requires: {count: 1}}]
`+tt.content)
			parsed, err := ParsePolicyWithParent(child, parent, nil)
			if tt.wantError {
				require.ErrorContains(t, err, "required slots are empty")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.comments, extensionComments(parsed))
			if tt.name == "omitted" {
				assert.Same(t, parent.ApprovalRules[0], parsed.FindApprovalRule("extra", "review"))
			} else if tt.name == "cleared" || tt.name == "null" {
				assert.Nil(t, parsed.FindApprovalRule("extra", "review"))
			}
		})
	}
}

func TestApprovalExtensionSelectedMethods(t *testing.T) {
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
    options: {methods: {comments: [unused]}}
`)
	child := extensionConfig(t, `
policy: {approval_extends: {remote: example/policies}}
slots: {extra: [{or: [review]}], unused: [unused]}
approval_rules:
  - name: review
    if: {has_labels: [service]}
    options: {methods: {comments: [child], github_review: false}}
    requires: {count: 1}
  - name: unused
    options: {methods: {comments: [unused]}}
`)
	parsed, err := ParsePolicyWithParent(child, parent, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"parent", "child"}, extensionComments(parsed))
	assert.Equal(t, common.TriggerCommit|common.TriggerComment|common.TriggerLabel, parsed.Trigger())
	// Standalone policies retain conservative audit coverage for unused rules.
	standalone, err := ParsePolicyWithParent(parent, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"parent", "replaced", "unused"}, extensionComments(standalone))
}

func TestApprovalExtensionInvalidSlotContents(t *testing.T) {
	for _, test := range []struct {
		name, slot string
		parent     bool
	}{
		{"replaced parent content", "extra", true},
		{"unused parent content", "unused", true},
		{"child content", "extra", false},
		{"unused child content", "unused", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := extensionConfig(t, "policy: {approval: [{slot: {name: extra}}]}")
			child := extensionConfig(t, "policy: {approval_extends: {remote: example/policies}}\nslots: {extra: []}")
			badContent := Slots{test.slot: {"missing"}}
			if test.parent {
				parent.Slots = badContent
			} else {
				child.Slots = badContent
			}
			_, err := ParsePolicyWithParent(child, parent, nil)
			require.ErrorContains(t, err, "undefined rule 'missing'")
		})
	}
	t.Run("undeclared parent default", func(t *testing.T) {
		parent := extensionConfig(t, `
policy: {approval: [{slot: {name: extra}}]}
slots: {unused: [review]}
approval_rules: [{name: review}]
`)
		child := extensionConfig(t, `policy: {approval_extends: {remote: example/policies}}`)
		_, err := ParsePolicyWithParent(child, parent, nil)
		require.ErrorContains(t, err, `slot content "unused" does not match a declared slot`)
	})
	child := extensionConfig(t, `policy: {approval_extends: {remote: example/policies}}`)
	_, err := ParsePolicyWithParent(child, child, nil)
	assert.ErrorContains(t, err, "parent cannot specify approval_extends")
	_, err = ParsePolicyWithParent(&Config{}, child, nil)
	assert.ErrorContains(t, err, "without approval_extends")
	parent := extensionConfig(t, `policy: {approval: [{slot: {name: extra, required: true}}]}`)
	_, err = ParsePolicyWithParent(child, parent, nil)
	assert.ErrorContains(t, err, "required slots are empty")

	for _, contentOwner := range []string{"parent", "child"} {
		t.Run("foreign rule in "+contentOwner+" content", func(t *testing.T) {
			parent := extensionConfig(t, `policy: {approval: [{slot: {name: extra}}]}`)
			child := extensionConfig(t, `policy: {approval_extends: {remote: example/policies}}`)
			if contentOwner == "parent" {
				parent.Slots = Slots{"extra": {"foreign"}}
				child.ApprovalRules = []*approval.Rule{{Name: "foreign"}}
			} else {
				child.Slots = Slots{"extra": {"foreign"}}
				parent.ApprovalRules = []*approval.Rule{{Name: "foreign"}}
			}
			_, err := ParsePolicyWithParent(child, parent, nil)
			assert.ErrorContains(t, err, "undefined rule 'foreign'")
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
  approval_extends: {remote: example/policies}
  disapproval:
    requires: {users: [child]}
    options:
      methods:
        disapprove: {comments: [child-block]}
        revoke: {comments: [child-clear]}
`)
	parsed, err := ParsePolicyWithParent(child, parent, nil)
	require.NoError(t, err)
	comments := extensionComments(parsed)
	assert.NotContains(t, comments, "parent-block")
	assert.Contains(t, comments, "child-block")
	assert.Contains(t, comments, "child-clear")
	prctx := &pulltest.Context{CommentsValue: []*pull.Comment{{Author: "parent", Body: "parent-block"}}}
	assert.Equal(t, common.StatusApproved, parsed.Evaluate(context.Background(), prctx).Status)
	prctx.CommentsValue = append(prctx.CommentsValue, &pull.Comment{Author: "child", Body: "child-block"})
	assert.Equal(t, common.StatusDisapproved, parsed.Evaluate(context.Background(), prctx).Status)
	prctx.CommentsValue = append(prctx.CommentsValue, &pull.Comment{Author: "child", Body: "child-clear", CreatedAt: time.Now()})
	assert.Equal(t, common.StatusApproved, parsed.Evaluate(context.Background(), prctx).Status)

	child.Policy.Disapproval = nil
	parsed, err = ParsePolicyWithParent(child, parent, nil)
	require.NoError(t, err)
	prctx.CommentsValue = []*pull.Comment{{Author: "parent", Body: "parent-block"}}
	assert.Equal(t, common.StatusApproved, parsed.Evaluate(context.Background(), prctx).Status)
	assert.NotContains(t, extensionComments(parsed), "parent-block")
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
policy: {approval_extends: {remote: example/policies}}
slots: {extra: [review]}
approval_rules: [{name: review, options: {ignore_edited_comments: false}}]
`)
	parsed, err := ParsePolicyWithParent(child, parent, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"parent", "server"}, extensionComments(parsed))
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
