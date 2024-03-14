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
	"context"

	"github.com/palantir/policy-bot/pull"
)

type Context struct {
	pull.Context
	ctx     context.Context
	options Options
}

func NewContext(ctx context.Context, pullContext pull.Context, options Options) *Context {
	return &Context{Context: pullContext, options: options}
}

func (c *Context) Comments() ([]*pull.Comment, error) {
	comments, err := c.Context.Comments()
	if err != nil {
		return nil, err
	}

	comments, err = c.filterIgnoredComments(c.Context, comments)
	if err != nil {
		return nil, err
	}

	comments = c.addApprovalComment(comments)
	return comments, nil
}

func (c *Context) filterIgnoredComments(prCtx pull.Context, comments []*pull.Comment) ([]*pull.Comment, error) {
	if c.options.IgnoreComments == nil {
		return comments, nil
	}

	var filteredComments []*pull.Comment
	for _, comment := range comments {
		isActor, err := c.options.IgnoreComments.IsActor(c.ctx, prCtx, comment.Author)
		if err != nil {
			return nil, err
		}

		if isActor {
			continue
		}

		filteredComments = append(filteredComments, comment)
	}

	return filteredComments, nil
}

func (c *Context) addApprovalComment(comments []*pull.Comment) []*pull.Comment {
	var commentsToAdd []*pull.Comment
	for _, comment := range c.options.AddComments {
		commentsToAdd = append(commentsToAdd, comment.toPullComment())
	}

	return append(comments, commentsToAdd...)
}

func (c *Context) Reviews() ([]*pull.Review, error) {
	reviews, err := c.Context.Reviews()
	if err != nil {
		return nil, err
	}

	reviews, err = c.filterIgnoredReviews(c.Context, reviews)
	if err != nil {
		return nil, err
	}

	reviews = c.addApprovalReview(reviews)
	return reviews, nil
}

func (c *Context) filterIgnoredReviews(prCtx pull.Context, reviews []*pull.Review) ([]*pull.Review, error) {
	if c.options.IgnoreReviews == nil {
		return reviews, nil
	}

	var filteredReviews []*pull.Review
	for _, review := range reviews {
		isActor, err := c.options.IgnoreReviews.IsActor(c.ctx, prCtx, review.Author)
		if err != nil {
			return nil, err
		}

		if isActor {
			continue
		}

		filteredReviews = append(filteredReviews, review)
	}

	return filteredReviews, nil
}

func (c *Context) addApprovalReview(reviews []*pull.Review) []*pull.Review {
	var reviewsToAdd []*pull.Review
	for _, review := range c.options.AddReviews {
		reviewsToAdd = append(reviewsToAdd, review.toPullReview())
	}

	return append(reviews, reviewsToAdd...)
}

func (c *Context) Branches() (string, string) {
	base, head := c.Context.Branches()
	if c.options.BaseBranch != "" {
		return c.options.BaseBranch, head
	}

	return base, head
}
