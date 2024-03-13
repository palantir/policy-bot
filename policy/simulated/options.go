// Copyright 2018 Palantir Technologies, Inc.
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

package simulated

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// Options should contain optional data that can be used to modify the results of the methods on the simulated Context.
type Options struct {
	IgnoreComments *common.Actors `json:"ignore_comments"`
	IgnoreReviews  *common.Actors `json:"ignore_reviews"`
	AddComments    []pull.Comment `json:"add_comments"`
	AddReviews     []pull.Review  `json:"add_reviews"`
	BaseBranch     string         `json:"base_branch"`
}

func NewOptionsFromRequest(r *http.Request) (Options, error) {
	var o Options
	if r.Body == nil {
		return o, nil
	}

	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		return o, errors.Wrap(err, "failed to unmarshal body into options")
	}

	o.setDefaults()
	return o, nil
}

// setDefaults sets any values for the options that were not intentionally set in the request body but which should have
// consistent values for the length of the simulation, such as the created time for a comment or review.
func (o *Options) setDefaults() {
	now := time.Now()
	for i, review := range o.AddReviews {
		if review.CreatedAt.IsZero() {
			review.CreatedAt = now
		}

		if review.LastEditedAt.IsZero() {
			review.LastEditedAt = now
		}

		review.ID = fmt.Sprintf("simulated-reviewID-%d", i)
		review.SHA = fmt.Sprintf("simulated-reviewSHA-%d", i)
		o.AddReviews[i] = review
	}

	for i, comment := range o.AddComments {
		if comment.CreatedAt.IsZero() {
			comment.CreatedAt = now
		}

		if comment.LastEditedAt.IsZero() {
			comment.LastEditedAt = now
		}

		o.AddComments[i] = comment
	}
}
