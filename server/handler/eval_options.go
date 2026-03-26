// Copyright 2022 Palantir Technologies, Inc.
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

package handler

import (
	"encoding"
	"os"
	"strconv"
	"strings"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
)

const (
	DefaultPolicyPath         = ".policy.yml"
	DefaultSharedRepository   = ".github"
	DefaultSharedPolicyPath   = "policy.yml"
	DefaultStatusCheckContext = "policy-bot"
)

type PullEvaluationOptions struct {
	PolicyPath string `yaml:"policy_path"`

	SharedRepository *string `yaml:"shared_repository"`
	SharedPolicyPath *string `yaml:"shared_policy_path"`

	// Ignore PolicyPath and use SharedRepository and SharedPolicyPath only.
	ForceSharedPolicy bool `yaml:"force_shared_policy"`

	// StatusCheckContext will be used to create the status context. It will be used in the following
	// pattern: <StatusCheckContext>: <Base Branch Name>
	StatusCheckContext string `yaml:"status_check_context"`

	// ExpandRequiredReviewers enables a UI feature where the details page
	// shows a list of the users who can approve each rule. Enabling this
	// feature can leak information about team membership and permissions that
	// is otherwise private. See the README for details.
	ExpandRequiredReviewers bool `yaml:"expand_required_reviewers"`

	// StrictReviewDismissal enables sending unconditional GitHub dismissals
	// for reviews associated with rules of invalidated approval candidates
	// even if that same approval candidate satifies another rule.
	StrictReviewDismissal bool `yaml:"strict_review_dismissal"`

	// PostInsecureStatusChecks enables the sending of a second status using just StatusCheckContext as the context,
	// no templating. This is turned off by default. This is to support legacy workflows that depend on the original
	// context behaviour, and will be removed in 2.0
	PostInsecureStatusChecks bool `yaml:"post_insecure_status_checks"`

	// IgnoreEditedComments enables ignoring comments that have been edited when evaluating approval rules.
	// This provides a server-side option to ignore edited comments across all rules.
	IgnoreEditedComments *bool `yaml:"ignore_edited_comments"`

	// ApprovalDefaults defines default values for all approval rules evaluated
	// by the server. Use this to change things like the default approval
	// comments or `invalidate_on_push` behavior globally. Policies may
	// override these default by providing their own values.
	ApprovalDefaults *approval.Defaults `yaml:"approval_defaults"`

	// This field is unused but is left to avoid breaking configuration files.
	// This value is now loaded from the GitHub API.
	//
	// TODO(bkeyes): remove in version 2.0
	Deprecated_AppName string `yaml:"app_name"`

	// This field is unused but is left to avoid breaking configuration files.
	// It enabled a temporary workaround for a GitHub API issue.
	//
	// TODO(bkeyes): remove in version 2.0
	Deprecated_DoNotLoadCommitPushedDate bool `yaml:"do_not_load_commit_pushed_date"`
}

func (p *PullEvaluationOptions) fillDefaults() {
	if p.PolicyPath == "" {
		p.PolicyPath = DefaultPolicyPath
	}
	if p.SharedRepository == nil {
		defaultSharedRepository := DefaultSharedRepository
		p.SharedRepository = &defaultSharedRepository
	}
	if p.SharedPolicyPath == nil {
		defaultSharedPolicyPath := DefaultSharedPolicyPath
		p.SharedPolicyPath = &defaultSharedPolicyPath
	}

	// Explicitly set either `SharedRepository` or `SharedPolicyPath` to an
	// empty string to disable the shared repository feature.
	if *p.SharedRepository == "" || *p.SharedPolicyPath == "" {
		emptyString := ""
		p.SharedRepository = &emptyString
		p.SharedPolicyPath = nil
	}

	if p.StatusCheckContext == "" {
		p.StatusCheckContext = DefaultStatusCheckContext
	}
}

func (p *PullEvaluationOptions) SetValuesFromEnv(prefix string) {
	setStringFromEnv("POLICY_PATH", prefix, &p.PolicyPath)
	setStringPtrFromEnv("SHARED_REPOSITORY", prefix, &p.SharedRepository)
	setStringPtrFromEnv("SHARED_POLICY_PATH", prefix, &p.SharedPolicyPath)
	setStringFromEnv("STATUS_CHECK_CONTEXT", prefix, &p.StatusCheckContext)
	setBoolFromEnv("FORCE_SHARED_POLICY", prefix, &p.ForceSharedPolicy)
	setBoolFromEnv("EXPAND_REQUIRED_REVIEWERS", prefix, &p.ExpandRequiredReviewers)
	setBoolFromEnv("STRICT_REVIEW_DISMISSAL", prefix, &p.StrictReviewDismissal)
	setBoolFromEnv("POST_INSECURE_STATUS_CHECKS", prefix, &p.PostInsecureStatusChecks)
	setBoolPtrFromEnv("IGNORE_EDITED_COMMENTS", prefix, &p.IgnoreEditedComments)

	p.setApprovalDefaultsFromEnv(prefix + "APPROVAL_DEFAULTS_")

	p.fillDefaults()
}

func (p *PullEvaluationOptions) setApprovalDefaultsFromEnv(prefix string) {
	var d *approval.Defaults
	if p.ApprovalDefaults != nil {
		d = p.ApprovalDefaults
	} else {
		d = &approval.Defaults{}
	}

	var opts *approval.Options
	if d.Options != nil {
		opts = d.Options
	} else {
		opts = &approval.Options{}
	}

	hasOpts := isAny(
		setBoolPtrFromEnv("OPTIONS_ALLOW_AUTHOR", prefix, &opts.AllowAuthor),
		setBoolPtrFromEnv("OPTIONS_ALLOW_CONTRIBUTOR", prefix, &opts.AllowContributor),
		setBoolPtrFromEnv("OPTIONS_ALLOW_NON_AUTHOR_CONTRIBUTOR", prefix, &opts.AllowNonAuthorContributor),
		setBoolPtrFromEnv("OPTIONS_INVALIDATE_ON_PUSH", prefix, &opts.InvalidateOnPush),
		setBoolPtrFromEnv("OPTIONS_IGNORE_EDITED_COMMENTS", prefix, &opts.IgnoreEditedComments),
		setBoolPtrFromEnv("OPTIONS_IGNORE_UPDATE_MERGES", prefix, &opts.IgnoreUpdateMerges),
	)

	var commitsBy *common.Actors
	if opts.IgnoreCommitsBy != nil {
		commitsBy = opts.IgnoreCommitsBy
	} else {
		commitsBy = &common.Actors{}
	}

	if isAny(
		setStringListFromEnv("OPTIONS_IGNORE_COMMITS_BY_USERS", prefix, &commitsBy.Users),
		setStringListFromEnv("OPTIONS_IGNORE_COMMITS_BY_TEAMS", prefix, &commitsBy.Teams),
		setStringListFromEnv("OPTIONS_IGNORE_COMMITS_BY_ORGANIZATIONS", prefix, &commitsBy.Organizations),
		setListFromEnv("OPTIONS_IGNORE_COMMITS_BY_PERMISSIONS", prefix, &commitsBy.Permissions),
	) {
		hasOpts = true
		if opts.IgnoreCommitsBy != commitsBy {
			opts.IgnoreCommitsBy = commitsBy
		}
	}

	var rr *approval.RequestReview
	if opts.RequestReview != nil {
		rr = opts.RequestReview
	} else {
		rr = &approval.RequestReview{}
	}

	if isAny(
		setBoolFromEnv("OPTIONS_REQUEST_REVIEW_ENABLED", prefix, &rr.Enabled),
		setStringFromEnv("OPTIONS_REQUEST_REVIEW_MODE", prefix, &rr.Mode),
		setIntFromEnv("OPTIONS_REQUEST_REVIEW_COUNT", prefix, &rr.Count),
	) {
		hasOpts = true
		if opts.RequestReview != rr {
			opts.RequestReview = rr
		}
	}

	var methods *common.Methods
	if opts.Methods != nil {
		methods = opts.Methods
	} else {
		methods = &common.Methods{}
	}

	if isAny(
		setStringListFromEnv("OPTIONS_METHODS_COMMENTS", prefix, &methods.Comments),
		setListFromEnv("OPTIONS_METHODS_COMMENT_PATTERNS", prefix, &methods.CommentPatterns),
		setBoolPtrFromEnv("OPTIONS_METHODS_GITHUB_REVIEW", prefix, &methods.GithubReview),
		setListFromEnv("OPTIONS_METHODS_GITHUB_REVIEW_COMMENT_PATTERNS", prefix, &methods.GithubReviewCommentPatterns),
		setListFromEnv("OPTIONS_METHODS_BODY_PATTERNS", prefix, &methods.BodyPatterns),
	) {
		hasOpts = true
		if opts.Methods != methods {
			opts.Methods = methods
		}
	}

	// Store new objects if we created them and set any properties
	if hasOpts {
		if d.Options != opts {
			d.Options = opts
		}
		if p.ApprovalDefaults != d {
			p.ApprovalDefaults = d
		}
	}
}

func isAny(bs ...bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}

func setStringFromEnv[T ~string](key, prefix string, value *T) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = T(v)
		return true
	}
	return false
}

func setStringPtrFromEnv[T ~string](key, prefix string, value **T) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		vt := T(v)
		*value = &vt
		return true
	}
	return false
}

func setStringListFromEnv[T ~string](key, prefix string, value *[]T) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		var items []T
		if v != "" {
			for item := range strings.SplitSeq(v, ",") {
				items = append(items, T(item))
			}
		} else {
			items = []T{} // set to an empty value requires a non-nil slice
		}
		*value = items
		return true
	}
	return false
}

type textUnmarshallerPtr[T any] interface {
	*T
	encoding.TextUnmarshaler
}

func setListFromEnv[T any, PT textUnmarshallerPtr[T]](key, prefix string, value *[]T) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		var items []T
		if v != "" {
			for item := range strings.SplitSeq(v, ",") {
				var t T
				if err := PT(&t).UnmarshalText([]byte(item)); err == nil {
					items = append(items, t)
				}
			}
		} else {
			items = []T{} // set to an empty value requires a non-nil slice
		}
		*value = items
		return true
	}
	return false
}

func setBoolFromEnv(key, prefix string, value *bool) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*value = b
			return true
		}
	}
	return false
}

func setBoolPtrFromEnv(key, prefix string, value **bool) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*value = &b
			return true
		}
	}
	return false
}

func setIntFromEnv(key, prefix string, value *int) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			*value = i
			return true
		}
	}
	return false
}
