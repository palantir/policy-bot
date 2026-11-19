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

package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"gopkg.in/yaml.v2"
)

// ConfigLoader allows ConfigFetcher to unit test policy loading decisions
type ConfigLoader interface {
	LoadConfig(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error)
	LoadRemoteRef(ctx context.Context, client *github.Client, clientOwner string, remote appconfig.RemoteRef) (appconfig.Config, error)
}

type PolicyLocation struct {
	Source string
	Path   string
}

type FetchedConfig struct {
	Config     *policy.Config
	Policy     *policy.ParsedPolicy
	LoadError  error
	ParseError error

	Source string
	Path   string
	Chain  []PolicyLocation

	SeenPolicy bool
}

func (fc FetchedConfig) policyLocation() string {
	location := fmt.Sprintf("policy %q in %q", fc.Path, fc.Source)
	for _, parent := range fc.Chain[1:] {
		if parent.Source != "" {
			location += fmt.Sprintf(" extending parent %q in %q", parent.Path, parent.Source)
		}
	}
	return location
}

type ConfigFetcher struct {
	Loader  ConfigLoader
	Options *policy.GlobalOptions
	// DefaultParentPath remains available when local policy discovery is disabled.
	DefaultParentPath string
	SeenPolicyCache   *SeenPolicyCache
}

// A policy can extend exactly one other policy.
const maxExtendsDepth = 1

func (cf *ConfigFetcher) ConfigForRepositoryBranch(ctx context.Context, client *github.Client, owner, repository, branch string) FetchedConfig {
	key := SeenPolicyKey{Owner: owner, Repository: repository, BaseBranch: branch}
	c, err := loadConfigWithRetry(ctx, func() (appconfig.Config, error) {
		return cf.Loader.LoadConfig(ctx, client, owner, repository, branch)
	})
	fc := FetchedConfig{Source: c.Source, Path: c.Path}
	if err != nil {
		fc.SeenPolicy = cf.SeenPolicyCache.Get(key)
		fc.LoadError = err
		return fc
	}
	if c.IsUndefined() {
		return fc
	}
	cf.SeenPolicyCache.Set(key)
	// Mark the branch as having seen a policy even if parsing or parent loading fails.
	fc.SeenPolicy = true
	fc.Chain = append(fc.Chain, PolicyLocation{Source: c.Source, Path: c.Path})
	var pc policy.Config
	if err := yaml.UnmarshalStrict(c.Content, &pc); err != nil {
		fc.ParseError = fmt.Errorf("failed to parse policy %q in %q: %w", c.Path, c.Source, err)
		return fc
	}
	fc.Config = &pc
	cur, err := policy.Parse(&pc, cf.Options)
	if err != nil {
		fc.ParseError = fmt.Errorf("failed to parse policy %q in %q: %w", c.Path, c.Source, err)
		return fc
	}
	for depth := 0; cur.Extends() != nil; depth++ {
		if depth >= maxExtendsDepth {
			fc.ParseError = fmt.Errorf("%s: parent policy must not extend another policy", fc.policyLocation())
			return fc
		}
		remote := *cur.Extends()
		if remote.Path == "" {
			remote.Path = cf.DefaultParentPath
		}
		parent, err := loadConfigWithRetry(ctx, func() (appconfig.Config, error) {
			// The supplied client belongs to owner even when the child came from another owner.
			return cf.Loader.LoadRemoteRef(ctx, client, owner, remote)
		})
		fc.Chain = append(fc.Chain, PolicyLocation{Source: parent.Source, Path: parent.Path})
		if err != nil {
			if parent.IsUndefined() {
				fc.LoadError = fmt.Errorf("failed to load parent policy requested by %q in %q: %w", c.Path, c.Source, err)
			} else {
				fc.LoadError = fmt.Errorf("failed to load parent policy %q in %q: %w", parent.Path, parent.Source, err)
			}
			return fc
		}
		if parent.IsUndefined() {
			fc.LoadError = fmt.Errorf("parent policy in %q does not exist", remote.Remote)
			return fc
		}
		var parentConfig policy.Config
		if err := yaml.UnmarshalStrict(parent.Content, &parentConfig); err != nil {
			fc.ParseError = fmt.Errorf("failed to parse parent policy %q in %q: %w", parent.Path, parent.Source, err)
			return fc
		}
		parsed, err := policy.Parse(&parentConfig, cf.Options)
		if err != nil {
			fc.ParseError = fmt.Errorf("failed to parse parent policy %q in %q: %w", parent.Path, parent.Source, err)
			return fc
		}
		cur = parsed.Extend(cur)
	}
	resolved, err := cur.Resolve()
	if err != nil {
		fc.ParseError = fmt.Errorf("%s: %w", fc.policyLocation(), err)
		return fc
	}
	fc.Policy = resolved
	return fc
}

func loadConfigWithRetry(ctx context.Context, load func() (appconfig.Config, error)) (appconfig.Config, error) {
	delay := time.Second
	for retries := 0; ; retries++ {
		c, err := load()
		retryable := os.IsTimeout(err) || isServerError(err)
		if !retryable || retries == 3 {
			return c, err
		}
		select {
		case <-ctx.Done():
			return c, ctx.Err()
		case <-time.After(delay):
			delay *= 2
		}
	}
}

func isServerError(err error) bool {
	if ghErr, ok := errors.AsType[*github.ErrorResponse](err); ok {
		return ghErr.Response.StatusCode >= 500 && ghErr.Response.StatusCode <= 599
	}
	return false
}
