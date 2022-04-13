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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
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
			&common.PredicateInfo{
				Type: "HasSuccessfulStatus",
				Name: "Status",
				StatusInfo: &common.StatusInfo{
					Status: []string{"status-name", "status-name-2"},
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
			&common.PredicateInfo{
				Type: "HasSuccessfulStatus",
				Name: "Status",
				StatusInfo: &common.StatusInfo{
					Status: []string{"status-name-2"},
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
			&common.PredicateInfo{
				Type: "HasSuccessfulStatus",
				Name: "Status",
				StatusInfo: &common.StatusInfo{
					Status: []string{"status-name", "status-name-2"},
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
			&common.PredicateInfo{
				Type: "HasSuccessfulStatus",
				Name: "Status",
				StatusInfo: &common.StatusInfo{
					Status: []string{"status-name-2"},
				},
			},
		},
		{
			"multiple statuses do not exist",
			false,
			&pulltest.Context{},
			&common.PredicateInfo{
				Type: "HasSuccessfulStatus",
				Name: "Status",
				StatusInfo: &common.StatusInfo{
					Status: []string{"status-name", "status-name-2"},
				},
			},
		},
	})
}

type StatusTestCase struct {
	name                  string
	expected              bool
	context               pull.Context
	ExpectedPredicateInfo *common.PredicateInfo
}

func runStatusTestCase(t *testing.T, p Predicate, cases []StatusTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, predicateInfo, err := p.Evaluate(ctx, tc.context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.expected, ok, "predicate was not correct")
				assert.Subset(t, tc.ExpectedPredicateInfo.StatusInfo.Status, predicateInfo.StatusInfo.Status, "StatusInfo was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Name, predicateInfo.Name, "PredicateInfo's Name was not correct")
				assert.Equal(t, tc.ExpectedPredicateInfo.Type, predicateInfo.Type, "PredicateInfo's Type was not correct")
			}
		})
	}
}
