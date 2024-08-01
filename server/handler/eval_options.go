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
	"os"
	"strconv"
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

	// StatusCheckContext will be used to create the status context. It will be used in the following
	// pattern: <StatusCheckContext>: <Base Branch Name>
	StatusCheckContext string `yaml:"status_check_context"`

	// ExpandRequiredReviewers enables a UI feature where the details page
	// shows a list of the users who can approve each rule. Enabling this
	// feature can leak information about team membership and permissions that
	// is otherwise private. See the README for details.
	ExpandRequiredReviewers bool `yaml:"expand_required_reviewers"`

	// PostInsecureStatusChecks enables the sending of a second status using just StatusCheckContext as the context,
	// no templating. This is turned off by default. This is to support legacy workflows that depend on the original
	// context behaviour, and will be removed in 2.0
	PostInsecureStatusChecks bool `yaml:"post_insecure_status_checks"`

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
	setBoolFromEnv("EXPAND_REQUIRED_REVIEWERS", prefix, &p.ExpandRequiredReviewers)
	setBoolFromEnv("POST_INSECURE_STATUS_CHECKS", prefix, &p.PostInsecureStatusChecks)
	p.fillDefaults()
}

func setStringFromEnv(key, prefix string, value *string) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = v
		return true
	}
	return false
}

func setStringPtrFromEnv(key, prefix string, value **string) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = &v
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
