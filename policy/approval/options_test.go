// Copyright 2025 Palantir Technologies, Inc.
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

package approval

import (
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/stretchr/testify/assert"
)

func TestOptionsFields(t *testing.T) {
	fields := map[string]struct {
		Seed         Options
		Get          func(Options) any
		UnsetValue   any
		SetValue     any
		DefaultValue any // use SetValue if nil
	}{
		"AllowAuthor": {
			Seed:       Options{AllowAuthor: new(true)},
			Get:        func(o Options) any { return o.IsAllowAuthor() },
			UnsetValue: false,
			SetValue:   true,
		},
		"AllowContributor": {
			Seed:       Options{AllowContributor: new(true)},
			Get:        func(o Options) any { return o.IsAllowContributor() },
			UnsetValue: false,
			SetValue:   true,
		},
		"AllowNonAuthorContributor": {
			Seed:       Options{AllowNonAuthorContributor: new(true)},
			Get:        func(o Options) any { return o.IsAllowNonAuthorContributor() },
			UnsetValue: false,
			SetValue:   true,
		},
		"InvalidateOnPush": {
			Seed:       Options{InvalidateOnPush: new(true)},
			Get:        func(o Options) any { return o.IsInvalidateOnPush() },
			UnsetValue: false,
			SetValue:   true,
		},
		"IgnoreEditedComments": {
			Seed:       Options{IgnoreEditedComments: new(true)},
			Get:        func(o Options) any { return o.IsIgnoreEditedComments() },
			UnsetValue: false,
			SetValue:   true,
		},
		"IgnoreUpdateMerges": {
			Seed:       Options{IgnoreUpdateMerges: new(true)},
			Get:        func(o Options) any { return o.IsIgnoreUpdateMerges() },
			UnsetValue: false,
			SetValue:   true,
		},
		"IgnoreCommitsBy": {
			Seed: Options{
				IgnoreCommitsBy: &common.Actors{
					Users: []string{"mhaypenny"},
				},
			},
			Get:        func(o Options) any { return o.GetIgnoreCommitsBy() },
			UnsetValue: common.Actors{},
			SetValue: common.Actors{
				Users: []string{"mhaypenny"},
			},
		},
		"RequestReview": {
			Seed: Options{
				RequestReview: &RequestReview{
					Enabled: true,
					Mode:    common.RequestModeTeams,
				},
			},
			Get:        func(o Options) any { return o.GetRequestReview() },
			UnsetValue: RequestReview{},
			SetValue: RequestReview{
				Enabled: true,
				Mode:    common.RequestModeTeams,
			},
		},
		"Methods": {
			Seed: Options{
				Methods: &common.Methods{
					Comments:     []string{"+1"},
					GithubReview: new(true),
				},
			},
			Get: func(o Options) any { return o.GetMethods() },
			UnsetValue: &common.Methods{
				GithubReviewState: pull.ReviewApproved,
			},
			SetValue: &common.Methods{
				Comments:          []string{"+1"},
				GithubReview:      new(true),
				GithubReviewState: pull.ReviewApproved,
			},
			DefaultValue: &common.Methods{
				GithubReviewState: pull.ReviewApproved,
				Defaults: &common.Methods{
					Comments:          []string{"+1"},
					GithubReview:      new(true),
					GithubReviewState: pull.ReviewApproved,
				},
			},
		},
	}

	for field, spec := range fields {
		t.Run(field, func(t *testing.T) {
			v := spec.Get(Options{})
			assert.Equal(t, spec.UnsetValue, v, "incorrect unset value")

			v = spec.Get(spec.Seed)
			assert.Equal(t, spec.SetValue, v, "incorrect set value")

			v = spec.Get(Options{Defaults: &spec.Seed})
			if spec.DefaultValue != nil {
				assert.Equal(t, spec.DefaultValue, v, "incorrect default value")
			} else {
				assert.Equal(t, spec.SetValue, v, "incorrect default value")
			}
		})
	}
}
