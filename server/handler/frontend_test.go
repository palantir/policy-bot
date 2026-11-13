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
	"html"
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailsReviewerSlotLinks(t *testing.T) {
	ec := extensionEvalContext(t, extensionPolicy+`
slots:
  service:
    - and: [Review, {or: [Review]}]
approval_rules:
  - name: Review
    requires: {count: 1, users: [child-reviewer]}
`, `
policy:
  approval: [Review, {slot: {name: service}}, {slot: {name: default}}, After]
slots:
  default: [{or: [Review]}]
approval_rules:
  - name: Review
    requires: {count: 1, users: [parent-reviewer]}
  - name: After
    requires: {count: 1, users: [other-reviewer]}
`)
	parsed, err := ec.ParseConfig(context.Background(), common.TriggerAll)
	require.NoError(t, err)
	result := parsed.Evaluate(context.Background(), ec.PullContext)
	require.NoError(t, result.Error)
	templates, err := LoadTemplates(&FilesConfig{Templates: "../templates", Static: t.TempDir()}, "", "https://github.com")
	require.NoError(t, err)
	data := map[string]any{
		"BasePath": "", "ExpandRequiredReviewers": true,
		"PullRequest": &github.PullRequest{Number: new(1), Base: &github.PullRequestBranch{Repo: &github.Repository{FullName: new("example/service")}}},
	}
	var body bytes.Buffer
	require.NoError(t, templates["details.html.tmpl"].ExecuteTemplate(&body, "results", []any{data, &result, ""}))
	for target, count := range map[string]int{
		"/details/example/service/1/reviewers?rule=Review":              1,
		"/details/example/service/1/reviewers?rule=Review&slot=service": 2,
		"/details/example/service/1/reviewers?rule=Review&slot=default": 1,
		"/details/example/service/1/reviewers?rule=After":               1,
	} {
		assert.Equal(t, count, strings.Count(html.UnescapeString(body.String()), `"`+target+`"`), target)
	}
}
