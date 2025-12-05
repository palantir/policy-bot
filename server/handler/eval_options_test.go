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

package handler

import (
	"regexp"
	"testing"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/stretchr/testify/assert"
)

func TestPullEvaluationOptions_SetValuesFromEnv(t *testing.T) {
	tests := map[string]struct {
		Env         map[string]string
		SetExpected func(*PullEvaluationOptions)
	}{
		"PolicyPath": {
			Env: map[string]string{"PEO_POLICY_PATH": "test/policy.yml"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.PolicyPath = "test/policy.yml"
			},
		},
		"SharedRepository": {
			Env: map[string]string{"PEO_SHARED_REPOSITORY": "settings"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.SharedRepository = ptr("settings")
			},
		},
		"SharedPolicyPath": {
			Env: map[string]string{"PEO_SHARED_POLICY_PATH": "configs/policy-bot/policy.yml"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.SharedPolicyPath = ptr("configs/policy-bot/policy.yml")
			},
		},
		"StatusCheckContext": {
			Env: map[string]string{"PEO_STATUS_CHECK_CONTEXT": "custom-policy-bot"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.StatusCheckContext = "custom-policy-bot"
			},
		},
		"ForceSharedPolicy": {
			Env: map[string]string{"PEO_FORCE_SHARED_POLICY": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ForceSharedPolicy = true
			},
		},
		"ExpandRequiredReviewers": {
			Env: map[string]string{"PEO_EXPAND_REQUIRED_REVIEWERS": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ExpandRequiredReviewers = true
			},
		},
		"StrictReviewDismissal": {
			Env: map[string]string{"PEO_STRICT_REVIEW_DISMISSAL": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.StrictReviewDismissal = true
			},
		},
		"PostInsecureStatusChecks": {
			Env: map[string]string{"PEO_POST_INSECURE_STATUS_CHECKS": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.PostInsecureStatusChecks = true
			},
		},
		"IgnoreEditedComments": {
			Env: map[string]string{"PEO_IGNORE_EDITED_COMMENTS": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.IgnoreEditedComments = ptr(true)
			},
		},
		"ApprovalDefaults.Options.AllowAuthor": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_ALLOW_AUTHOR": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						AllowAuthor: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.AllowContributor": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_ALLOW_CONTRIBUTOR": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						AllowContributor: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.AllowNonAuthorContributor": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_ALLOW_NON_AUTHOR_CONTRIBUTOR": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						AllowNonAuthorContributor: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.InvalidateOnPush": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_INVALIDATE_ON_PUSH": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						InvalidateOnPush: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreEditedComments": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_EDITED_COMMENTS": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreEditedComments: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreUpdateMerges": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_UPDATE_MERGES": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreUpdateMerges: ptr(true),
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Users": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_USERS": "bot-user,ci-user"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Users: []string{"bot-user", "ci-user"},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Users_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_USERS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Users: []string{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Teams": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_TEAMS": "org/bots,org/ci"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Teams: []string{"org/bots", "org/ci"},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Teams_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_TEAMS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Teams: []string{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Organizations": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_ORGANIZATIONS": "bot-org,automation-org"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Organizations: []string{"bot-org", "automation-org"},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Organizations_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_ORGANIZATIONS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Organizations: []string{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Permissions": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_PERMISSIONS": "admin,write"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Permissions: []pull.Permission{pull.PermissionAdmin, pull.PermissionWrite},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.IgnoreCommitsBy.Permissions_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_IGNORE_COMMITS_BY_PERMISSIONS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						IgnoreCommitsBy: &common.Actors{
							Permissions: []pull.Permission{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.RequestReview.Enabled": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_REQUEST_REVIEW_ENABLED": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						RequestReview: &approval.RequestReview{
							Enabled: true,
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.RequestReview.Mode": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_REQUEST_REVIEW_MODE": "random"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						RequestReview: &approval.RequestReview{
							Mode: "random",
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.RequestReview.Count": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_REQUEST_REVIEW_COUNT": "3"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						RequestReview: &approval.RequestReview{
							Count: 3,
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.Comments": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_COMMENTS": "LGTM,approved"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							Comments: []string{"LGTM", "approved"},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.Comments_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_COMMENTS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							Comments: []string{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.CommentPatterns": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_COMMENT_PATTERNS": "^looks good,^approved by"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							CommentPatterns: []common.Regexp{
								common.NewCompiledRegexp(regexp.MustCompile("^looks good")),
								common.NewCompiledRegexp(regexp.MustCompile("^approved by")),
							},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.CommentPatterns_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_COMMENT_PATTERNS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							CommentPatterns: []common.Regexp{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.GithubReview": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_GITHUB_REVIEW": "true"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							GithubReview: ptr(true),
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.GithubReviewCommentPatterns": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_GITHUB_REVIEW_COMMENT_PATTERNS": "^approved,^LGTM"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							GithubReviewCommentPatterns: []common.Regexp{
								common.NewCompiledRegexp(regexp.MustCompile("^approved")),
								common.NewCompiledRegexp(regexp.MustCompile("^LGTM")),
							},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.GithubReviewCommentPatterns_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_GITHUB_REVIEW_COMMENT_PATTERNS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							GithubReviewCommentPatterns: []common.Regexp{},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.BodyPatterns": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_BODY_PATTERNS": "(?m)^Approved-By:,(?m)^Signed-Off-By:"},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							BodyPatterns: []common.Regexp{
								common.NewCompiledRegexp(regexp.MustCompile("(?m)^Approved-By:")),
								common.NewCompiledRegexp(regexp.MustCompile("(?m)^Signed-Off-By:")),
							},
						},
					},
				}
			},
		},
		"ApprovalDefaults.Options.Methods.BodyPatterns_Empty": {
			Env: map[string]string{"PEO_APPROVAL_DEFAULTS_OPTIONS_METHODS_BODY_PATTERNS": ""},
			SetExpected: func(opts *PullEvaluationOptions) {
				opts.ApprovalDefaults = &approval.Defaults{
					Options: &approval.Options{
						Methods: &common.Methods{
							BodyPatterns: []common.Regexp{},
						},
					},
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for k, v := range test.Env {
				t.Setenv(k, v)
			}

			opts := PullEvaluationOptions{}
			opts.SetValuesFromEnv("PEO_")

			// Explicitly set defaults to avoid calling `fillDefaults` on both
			// the input and the output and hiding potential bugs
			expected := PullEvaluationOptions{
				PolicyPath:         DefaultPolicyPath,
				SharedRepository:   ptr(DefaultSharedRepository),
				SharedPolicyPath:   ptr(DefaultSharedPolicyPath),
				StatusCheckContext: DefaultStatusCheckContext,
			}
			test.SetExpected(&expected)

			assert.Equal(t, expected, opts, "incorrect options set from environment")
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
