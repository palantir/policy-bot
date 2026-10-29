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
	"bytes"
	"context"
	"encoding/json"
	"html"
	"html/template"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailsSlots(t *testing.T) {
	const slotName = `service <img src=x onerror="alert(1)">`
	const ruleName = `Service & "review"?+#`
	ec := slotEvalContext(t, `
policy:
  approval:
    - slot: empty-top
    - slot: skipped
    - slot: 'service <img src=x onerror="alert(1)">'
    - or:
        - Nested review
        - slot: empty-nested
    - Already approved
slots:
  skipped: [Skipped rule]
  'service <img src=x onerror="alert(1)">': ['Service & "review"?+#']
approval_rules:
  - name: Skipped rule
    if:
      targets_branch:
        pattern: '^release$'
  - name: 'Service & "review"?+#'
    requires:
      count: 1
      users: [alice]
  - name: Nested review
    requires:
      count: 1
      users: [bob]
  - name: Already approved
`)
	eval, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.NoError(t, err)
	result := eval.Evaluate(context.Background(), ec.PullContext)
	require.NoError(t, result.Error)
	require.Equal(t, common.StatusPending, result.Status)
	original, err := json.Marshal(result)
	require.NoError(t, err)

	templates, err := LoadTemplates(&FilesConfig{Templates: "../templates", Static: t.TempDir()}, "/policy-bot", "https://github.com")
	require.NoError(t, err)
	data := map[string]any{
		"BasePath":                "/policy-bot",
		"ExpandRequiredReviewers": true,
		"Result":                  &result,
		"PullRequest": &github.PullRequest{
			Number: new(123),
			Base: &github.PullRequestBranch{
				Repo: &github.Repository{FullName: new("example/service")},
			},
		},
	}
	h := Details{Templates: templates}
	response := httptest.NewRecorder()
	require.NoError(t, h.render(response, data))
	body := response.Body.String()

	assert.NotContains(t, body, "empty-top")
	assert.NotContains(t, body, "empty-nested")
	assert.Contains(t, body, "Slot: skipped")
	assert.Contains(t, body, "Skipped rule")
	assert.Contains(t, body, `data-hide-status="skipped"`)
	assert.Contains(t, body, "Slot: "+template.HTMLEscapeString(slotName))
	assert.NotContains(t, body, "<img")
	assert.Contains(t, html.UnescapeString(body), ruleName)
	assert.Less(t, strings.Index(body, "Already approved"), strings.Index(body, "Slot: service"))
	assert.Less(t, strings.Index(body, "Slot: service"), strings.Index(body, "Slot: skipped"))

	var linkedRules []string
	for _, match := range regexp.MustCompile(`hx-get="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		link, err := url.Parse(html.UnescapeString(match[1]))
		require.NoError(t, err)
		assert.Equal(t, "/policy-bot/details/example/service/123/reviewers", link.Path)
		assert.Len(t, link.Query(), 1)
		linkedRules = append(linkedRules, link.Query().Get("rule"))
	}
	assert.ElementsMatch(t, []string{ruleName, "Nested review"}, linkedRules)
	assert.Same(t, &ec.Config.Config.ApprovalRules[1].Requires, findRuleRequires(ec.Config.Config, ruleName))

	// Empty slots must not keep a rendered branch open.
	nested := result.Children[0].Children[3]
	require.Equal(t, "or", nested.Name)
	var branch bytes.Buffer
	require.NoError(t, templates["details.html.tmpl"].ExecuteTemplate(&branch, "results", []any{data, nested}))
	assert.Equal(t, 1, strings.Count(branch.String(), `<li class="node"`))
	assert.Contains(t, branch.String(), `<li class="node" data-status="pending" data-next-status="">`)

	after, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Equal(t, original, after, "rendering must not reorder or remove evaluation results")
}
