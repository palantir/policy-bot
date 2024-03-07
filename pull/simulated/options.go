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
	"time"

	"github.com/palantir/policy-bot/pull"
)

// Options should contain optional data that can be used to modify the results of the methods on the simulated Context.
type Options struct {
	Ignore             string
	AddApprovalComment string
	AddApprovalReview  string
	BaseBranch         string
}

func (o *Options) filterIgnoredComments(comments []*pull.Comment) []*pull.Comment {
	var filteredComments []*pull.Comment
	for _, comment := range comments {
		if comment.Author == o.Ignore {
			continue
		}

		filteredComments = append(filteredComments, comment)
	}

	return filteredComments
}

func (o *Options) filterIgnoredReviews(reviews []*pull.Review) []*pull.Review {
	var filteredReviews []*pull.Review
	for _, review := range reviews {
		if review.Author == o.Ignore {
			continue
		}

		filteredReviews = append(filteredReviews, review)
	}

	return filteredReviews
}

func (o *Options) addApprovalComment(comments []*pull.Comment) []*pull.Comment {
	return append(comments, &pull.Comment{
		CreatedAt:    time.Now(),
		LastEditedAt: time.Now(),
		Author:       o.AddApprovalComment,
		Body:         ":+1:",
	})
}

func (o *Options) addApprovalReview(reviews []*pull.Review) []*pull.Review {
	return append(reviews, &pull.Review{
		CreatedAt:    time.Now(),
		LastEditedAt: time.Now(),
		Author:       o.AddApprovalReview,
		State:        pull.ReviewApproved,
	})
}
