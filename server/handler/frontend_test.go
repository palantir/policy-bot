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
	"fmt"
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/stretchr/testify/require"
)

func TestDetailsTreeErrorConnections(t *testing.T) {
	templates, err := LoadTemplates(&FilesConfig{Templates: "../templates", Static: t.TempDir()}, "", "https://github.com")
	require.NoError(t, err)

	for _, status := range []common.EvaluationStatus{common.StatusSkipped, common.StatusPending} {
		t.Run(status.String(), func(t *testing.T) {
			result := &common.Result{Children: []*common.Result{
				{Name: "active", Status: common.StatusApproved},
				{Name: "skipped", Status: common.StatusSkipped},
				{Name: "error", Status: status, Error: fmt.Errorf("evaluation failed")},
				{Name: "another-error", Status: status, Error: fmt.Errorf("another evaluation failed")},
				{Name: "another-skipped", Status: common.StatusSkipped},
			}}
			var output bytes.Buffer
			root := struct{ ExpandRequiredReviewers bool }{}
			require.NoError(t, templates["details.html.tmpl"].ExecuteTemplate(&output, "results", []any{root, result}))
			nodes := regexp.MustCompile(`<li class="node"[^>]*>`).FindAllString(output.String(), -1)
			require.Equal(t, []string{
				`<li class="node" data-status="error" >`,
				`<li class="node" data-status="error" data-next-status="approved">`,
				`<li class="node" data-status="approved" data-next-status="skipped">`,
				`<li class="node" data-status="skipped" >`,
				`<li class="node" data-status="skipped" data-next-status="">`,
			}, nodes)
		})
	}
}
