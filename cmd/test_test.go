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

package cmd

import (
	"context"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/pull"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const testPolicy = `
policy:
  approval:
  - a reviewer approves
approval_rules:
- name: a reviewer approves
  requires:
    count: 1
    teams:
    - "testorg/reviewers"
`

func TestScenarioEvaluation(t *testing.T) {
	var cfg policy.Config
	require.NoError(t, yaml.UnmarshalStrict([]byte(testPolicy), &cfg))
	eval, err := policy.ParsePolicy(&cfg, nil)
	require.NoError(t, err)

	cases := []struct {
		name string
		sc   scenario
		want string
	}{
		{
			"no approvals is pending",
			scenario{Author: "alice"},
			"pending",
		},
		{
			"author cannot self-approve",
			scenario{
				Author:      "alice",
				TeamMembers: map[string][]string{"alice": {"testorg/reviewers"}},
				Reviews:     []scenarioReview{{Author: "alice", State: "approved"}},
			},
			"pending",
		},
		{
			"team member review approves",
			scenario{
				Author:      "alice",
				TeamMembers: map[string][]string{"bob": {"testorg/reviewers"}},
				Reviews:     []scenarioReview{{Author: "bob", State: "approved"}},
			},
			"approved",
		},
		{
			"non-member review stays pending",
			scenario{
				Author:  "alice",
				Reviews: []scenarioReview{{Author: "carol", State: "approved"}},
			},
			"pending",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := eval.Evaluate(context.Background(), tc.sc.toContext())
			require.NoError(t, res.Error)
			assert.Equal(t, tc.want, res.Status.String())
		})
	}
}

func TestScenarioToContext(t *testing.T) {
	sc := scenario{
		Author:       "alice",
		Labels:       []string{"enhancement"},
		ChangedFiles: []scenarioFile{{Name: "a.go", Status: "added", Additions: 3}},
		Comments:     []scenarioComment{{Author: "bob", Body: ":+1:"}},
		Reviews:      []scenarioReview{{Author: "bob", State: "approved", SHA: "abc123"}},
	}
	ctx := sc.toContext()

	assert.Equal(t, "alice", ctx.Author())
	assert.True(t, ctx.IsOpen(), "state should default to open")

	labels, err := ctx.Labels()
	require.NoError(t, err)
	assert.Equal(t, []string{"enhancement"}, labels)

	files, err := ctx.ChangedFiles()
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "a.go", files[0].Filename)
	assert.Equal(t, pull.FileAdded, files[0].Status)
	assert.Equal(t, 3, files[0].Additions)

	comments, err := ctx.Comments()
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "bob", comments[0].Author)

	reviews, err := ctx.Reviews()
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, pull.ReviewApproved, reviews[0].State)
	assert.Equal(t, "abc123", reviews[0].SHA)
}

func TestLoadFixtures(t *testing.T) {
	fixtures, err := loadFixtures("testdata/fixtures.yml")
	require.NoError(t, err)
	assert.NotEmpty(t, fixtures.Scenarios)
	for _, sc := range fixtures.Scenarios {
		assert.True(t, validExpectations[sc.Expected], "scenario %q has a valid expectation", sc.Name)
	}
}
