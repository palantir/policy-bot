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
	"strings"
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

func (p PolicyLocation) String() string {
	return fmt.Sprintf("%q in %q", p.Path, p.Source)
}

type FetchedConfig struct {
	Config     *policy.Config
	Policy     *policy.ParsedPolicy
	LoadError  error
	ParseError error

	// Sources starts with the leaf lookup, even if no policy was found.
	Sources []PolicyLocation

	SeenPolicy bool
}

func (fc FetchedConfig) PolicyLocation() string {
	locations := make([]string, len(fc.Sources))
	for i, source := range fc.Sources {
		locations[i] = source.String()
	}
	return "policy " + strings.Join(locations, " extending parent ")
}

type ConfigFetcher struct {
	Loader          ConfigLoader
	Options         *policy.GlobalOptions
	SeenPolicyCache *SeenPolicyCache
}

// A policy can extend exactly one other policy.
const maxExtendsDepth = 1

func (cf *ConfigFetcher) ConfigForRepositoryBranch(ctx context.Context, client *github.Client, owner, repository, branch string) FetchedConfig {
	key := SeenPolicyKey{Owner: owner, Repository: repository, BaseBranch: branch}
	c, err := loadConfigWithRetry(ctx, func() (appconfig.Config, error) {
		return cf.Loader.LoadConfig(ctx, client, owner, repository, branch)
	})
	source := PolicyLocation{Source: c.Source, Path: c.Path}
	fc := FetchedConfig{Sources: []PolicyLocation{source}}
	if err != nil {
		fc.SeenPolicy = cf.SeenPolicyCache.Get(key)
		fc.LoadError = err
		return fc
	}
	if c.IsUndefined() {
		return fc
	}
	// Mark the branch as having seen a policy even if parsing or parent loading fails.
	cf.SeenPolicyCache.Set(key)
	fc.SeenPolicy = true

	var pc policy.Config
	if err := yaml.UnmarshalStrict(c.Content, &pc); err != nil {
		fc.ParseError = fmt.Errorf("failed to parse policy %s: %w", source, err)
		return fc
	}
	fc.Config = &pc
	cur, err := policy.Parse(&pc, cf.Options)
	if err != nil {
		fc.ParseError = fmt.Errorf("failed to parse policy %s: %w", source, err)
		return fc
	}
	for depth := 0; cur.ApprovalExtends() != nil; depth++ {
		if depth >= maxExtendsDepth {
			fc.ParseError = fmt.Errorf("%s: parent policy must not extend another policy", fc.PolicyLocation())
			return fc
		}
		remote := *cur.ApprovalExtends()
		parent, err := loadConfigWithRetry(ctx, func() (appconfig.Config, error) {
			// The supplied client belongs to owner even when the child came from another owner.
			return cf.Loader.LoadRemoteRef(ctx, client, owner, remote)
		})
		parentSource := PolicyLocation{Source: parent.Source, Path: parent.Path}
		fc.Sources = append(fc.Sources, parentSource)
		if err != nil {
			fc.LoadError = fmt.Errorf("failed to load parent policy %s: %w", parentSource, err)
			return fc
		}
		var parentConfig policy.Config
		if err := yaml.UnmarshalStrict(parent.Content, &parentConfig); err != nil {
			fc.ParseError = fmt.Errorf("%s: failed to parse parent policy: %w", fc.PolicyLocation(), err)
			return fc
		}
		parsed, err := policy.Parse(&parentConfig, cf.Options)
		if err != nil {
			fc.ParseError = fmt.Errorf("%s: failed to parse parent policy: %w", fc.PolicyLocation(), err)
			return fc
		}
		cur = cur.Extend(parsed)
	}
	resolved, err := cur.Resolve()
	if err != nil {
		fc.ParseError = fmt.Errorf("%s: %w", fc.PolicyLocation(), err)
		return fc
	}
	fc.Policy = resolved
	return fc
}

func loadConfigWithRetry(ctx context.Context, load func() (appconfig.Config, error)) (appconfig.Config, error) {
	var c appconfig.Config
	err := withRetry(ctx, defaultRetryDelay, func() (err error) {
		c, err = load()
		return err
	})
	return c, err
}

const defaultRetryDelay = time.Second

// withRetry calls fn until it succeeds, returns an error that is not a timeout
// or server error, or fails after 3 retries. The delay before the first retry
// doubles after each retry.
func withRetry(ctx context.Context, delay time.Duration, fn func() error) error {
	for retries := 0; ; retries++ {
		err := fn()
		retryable := os.IsTimeout(err) || isServerError(err)
		if !retryable || retries == 3 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
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
