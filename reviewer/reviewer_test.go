// Copyright 2019 Palantir Technologies, Inc.
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

package reviewer

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/policy-bot/policy/common"
)

func Test_FindLeafResults(t *testing.T) {
	results := common.Result{
		Name:        "One",
		Description: "",
		Status:      common.StatusPending,
		Rule:        common.Rule{},
		Error:       nil,
		Children: []*common.Result{{
			Name:        "Two",
			Description: "",
			Status:      common.StatusPending,
			Rule:        common.Rule{},
			Error:       nil,
			Children:    nil,
		},
			{
				Name:        "Three",
				Description: "",
				Status:      common.StatusDisapproved,
				Rule:        common.Rule{},
				Error:       errors.New("foo"),
				Children:    nil,
			},
			{
				Name:        "Four",
				Description: "",
				Status:      common.StatusPending,
				Rule:        common.Rule{},
				Error:       nil,
				Children: []*common.Result{{
					Name:        "Five",
					Description: "",
					Status:      common.StatusPending,
					Rule:        common.Rule{},
					Error:       nil,
					Children:    nil,
				},
				},
			},
		},
	}

	actualResults := findLeafChildren(results)
	require.Len(t, actualResults, 2, "incorrect number of results")
}

func Test_selectRandomUsers(t *testing.T) {
	// (n int, users []string, r *rand.Rand) []string {
	r := rand.New(rand.NewSource(42))

	require.Len(t, selectRandomUsers(0, []string{"a"}, r), 0, "0 selection should return nothing")
	require.Len(t, selectRandomUsers(1, []string{}, r), 0, "empty list should return nothing")

	assert.Equal(t, []string{"a"}, selectRandomUsers(1, []string{"a"}, r))
	assert.Equal(t, []string{"a", "b"}, selectRandomUsers(3, []string{"a", "b"}, r))

	pseudoRandom := selectRandomUsers(1, []string{"a", "b", "c"}, r)
	assert.Equal(t, []string{"c"}, pseudoRandom)

	multiplePseudoRandom := selectRandomUsers(4, []string{"a", "b", "c", "d", "e", "f", "g"}, r)
	assert.Equal(t, []string{"c", "e", "b", "f"}, multiplePseudoRandom)
}
