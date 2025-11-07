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

func DefaultMethods() *common.Methods {
	review := true
	return &common.Methods{
		Comments: []string{
			":+1:",
			"👍",
		},
		GithubReview: &review,
	}
}

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

	// Defaults contains default options values for this rule, set by the
	// policy or the server. The field is populated after parsing the YAML
	// configuration. If nil, unset fields default to the zero value of their
	// element type, unless otherwise noted.
	Defaults *Options `yaml:"-"`
}

type RequestReview struct {
	Enabled bool               `yaml:"enabled,omitempty"`
	Mode    common.RequestMode `yaml:"mode,omitempty"`
	Count   int                `yaml:"count,omitempty"`
}

func (opts Options) IsAllowAuthor() bool {
	if opts.AllowAuthor == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsAllowAuthor()
		}
		return false
	}
	return *opts.AllowAuthor
}

func (opts Options) IsAllowContributor() bool {
	if opts.AllowContributor == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsAllowContributor()
		}
		return false
	}
	return *opts.AllowContributor
}

func (opts Options) IsAllowNonAuthorContributor() bool {
	if opts.AllowNonAuthorContributor == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsAllowNonAuthorContributor()
		}
		return false
	}
	return *opts.AllowNonAuthorContributor
}

func (opts Options) IsInvalidateOnPush() bool {
	if opts.InvalidateOnPush == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsInvalidateOnPush()
		}
		return false
	}
	return *opts.InvalidateOnPush
}

func (opts Options) IsIgnoreEditedComments() bool {
	if opts.IgnoreEditedComments == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsIgnoreEditedComments()
		}
		return false
	}
	return *opts.IgnoreEditedComments
}

func (opts Options) IsIgnoreUpdateMerges() bool {
	if opts.IgnoreUpdateMerges == nil {
		if opts.Defaults != nil {
			return opts.Defaults.IsIgnoreUpdateMerges()
		}
		return false
	}
	return *opts.IgnoreUpdateMerges
}

func (opts Options) GetIgnoreCommitsBy() common.Actors {
	if opts.IgnoreCommitsBy == nil {
		if opts.Defaults != nil {
			return opts.Defaults.GetIgnoreCommitsBy()
		}
		return common.Actors{}
	}
	return *opts.IgnoreCommitsBy
}

func (opts Options) GetRequestReview() RequestReview {
	if opts.RequestReview == nil {
		if opts.Defaults != nil {
			return opts.Defaults.GetRequestReview()
		}
		return RequestReview{}
	}
	return *opts.RequestReview
}

func (opts Options) GetMethods() *common.Methods {
	var m common.Methods
	if opts.Methods != nil {
		m = *opts.Methods // make a copy since we might modify 'm.Defaults'
	}
	if opts.Defaults != nil {
		m.Defaults = opts.Defaults.GetMethods()
	}

	// Approvals always use `pull.ReviewApproved` as the state
	m.GithubReviewState = pull.ReviewApproved
	return &m
}
