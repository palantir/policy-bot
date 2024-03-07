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
	"github.com/palantir/policy-bot/pull"
)

type Context struct {
	pull.Context
	options Options
}

func NewContext(pullContext pull.Context, options Options) *Context {
	return &Context{Context: pullContext, options: options}
}

func (c *Context) Comments() ([]*pull.Comment, error) {
	comments, err := c.Context.Comments()
	if err != nil {
		return nil, err
	}

	if c.options.Ignore != "" {
		comments = c.options.filterIgnoredComments(comments)
	}

	if c.options.AddApprovalComment != "" {
		comments = c.options.addApprovalComment(comments)
	}

	return comments, nil
}

func (c *Context) Reviews() ([]*pull.Review, error) {
	reviews, err := c.Context.Reviews()
	if err != nil {
		return nil, err
	}

	if c.options.Ignore != "" {
		reviews = c.options.filterIgnoredReviews(reviews)
	}

	if c.options.AddApprovalReview != "" {
		reviews = c.options.addApprovalReview(reviews)
	}

	return reviews, nil
}

func (c *Context) Branches() (string, string) {
	base, head := c.Context.Branches()
	if c.options.BaseBranch != "" {
		return c.options.BaseBranch, head
	}

	return base, head
}
