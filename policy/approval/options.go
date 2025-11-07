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
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type Options struct {
	AllowAuthor               *bool `yaml:"allow_author,omitempty"`
	AllowContributor          *bool `yaml:"allow_contributor,omitempty"`
	AllowNonAuthorContributor *bool `yaml:"allow_non_author_contributor,omitempty"`
	InvalidateOnPush          *bool `yaml:"invalidate_on_push,omitempty"`

	IgnoreEditedComments *bool          `yaml:"ignore_edited_comments,omitempty"`
	IgnoreUpdateMerges   *bool          `yaml:"ignore_update_merges,omitempty"`
	IgnoreCommitsBy      *common.Actors `yaml:"ignore_commits_by,omitempty"`

	RequestReview *RequestReview `yaml:"request_review,omitempty"`

	Methods *common.Methods `yaml:"methods,omitempty"`
}

type RequestReview struct {
	Enabled bool               `yaml:"enabled,omitempty"`
	Mode    common.RequestMode `yaml:"mode,omitempty"`
	Count   int                `yaml:"count,omitempty"`
}

func (opts *Options) IsAllowAuthor() bool {
	if opts.AllowAuthor == nil {
		return false
	}
	return *opts.AllowAuthor
}

func (opts *Options) IsAllowContributor() bool {
	if opts.AllowContributor == nil {
		return false
	}
	return *opts.AllowContributor
}

func (opts *Options) IsAllowNonAuthorContributor() bool {
	if opts.AllowNonAuthorContributor == nil {
		return false
	}
	return *opts.AllowNonAuthorContributor
}

func (opts *Options) IsInvalidateOnPush() bool {
	if opts.InvalidateOnPush == nil {
		return false
	}
	return *opts.InvalidateOnPush
}

func (opts *Options) IsIgnoreEditedComments() bool {
	if opts.IgnoreEditedComments == nil {
		return false
	}
	return *opts.IgnoreEditedComments
}

func (opts *Options) IsIgnoreUpdateMerges() bool {
	if opts.IgnoreUpdateMerges == nil {
		return false
	}
	return *opts.IgnoreUpdateMerges
}

func (opts *Options) GetIgnoreCommitsBy() common.Actors {
	if opts.IgnoreCommitsBy == nil {
		return common.Actors{}
	}
	return *opts.IgnoreCommitsBy
}

func (opts *Options) GetRequestReview() RequestReview {
	if opts.RequestReview == nil {
		return RequestReview{}
	}
	return *opts.RequestReview
}

func (opts *Options) GetMethods() *common.Methods {
	methods := opts.Methods
	if methods == nil {
		methods = &common.Methods{}
	}
	if methods.Comments == nil {
		methods.Comments = []string{
			":+1:",
			"👍",
		}
	}
	if methods.GithubReview == nil {
		defaultGithubReview := true
		methods.GithubReview = &defaultGithubReview
	}

	methods.GithubReviewState = pull.ReviewApproved
	return methods
}
