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

package predicate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
)

func TestHasSuccessfulStatus(t *testing.T) {
	p := HasSuccessfulStatus([]string{"status-name", "status-name-2"})

	runStatusTestCase(t, p, []StatusTestCase{
		{
			"all statuses succeed",
			true,
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "success",
					"status-name-2": "success",
				},
			},
		},
		{
			"a status fails",
			false,
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "success",
					"status-name-2": "failure",
				},
			},
		},
		{
			"multiple statuses fail",
			false,
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name":   "failure",
					"status-name-2": "failure",
				},
			},
		},
		{
			"a status does not exist",
			false,
			&pulltest.Context{
				LatestStatusesValue: map[string]string{
					"status-name": "success",
				},
			},
		},
		{
			"multiple statuses do not exist",
			false,
			&pulltest.Context{},
		},
	})
}

type StatusTestCase struct {
	name     string
	expected bool
	context  pull.Context
}

func runStatusTestCase(t *testing.T, p Predicate, cases []StatusTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, _, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.expected, ok, "predicate was not correct")
			}
		})
	}
}
