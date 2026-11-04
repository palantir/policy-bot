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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyBranch(t *testing.T) {
	off := &Base{PullOpts: &PullEvaluationOptions{}}
	branch, err := off.policyBranch(context.Background(), nil, "testorg", "testrepo", "feat-b", "main")
	require.NoError(t, err)
	assert.Equal(t, "feat-b", branch)

	on := &Base{PullOpts: &PullEvaluationOptions{PolicyFromDefaultBranch: true}}
	branch, err = on.policyBranch(context.Background(), nil, "testorg", "testrepo", "feat-b", "main")
	require.NoError(t, err)
	assert.Equal(t, "main", branch, "uses the payload default branch without an API call")
}
