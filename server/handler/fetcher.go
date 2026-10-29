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
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"gopkg.in/yaml.v2"
)

// ConfigLoader allows ConfigFetcher to unit test policy loading decisions
type ConfigLoader interface {
	LoadConfig(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error)
	LoadRemoteRef(ctx context.Context, client *github.Client, clientOwner string, remote appconfig.RemoteRef) (appconfig.Config, error)
}

type FetchedConfig struct {
	Config     *policy.Config
	Parent     *policy.Config
	LoadError  error
	ParseError error

	Source string
	Path   string

	SeenPolicy bool
}

type ConfigFetcher struct {
	Loader          ConfigLoader
	SeenPolicyCache *SeenPolicyCache
}

func (cf *ConfigFetcher) ConfigForRepositoryBranch(ctx context.Context, client *github.Client, owner, repository, branch string) FetchedConfig {
	key := SeenPolicyKey{
		Owner:      owner,
		Repository: repository,
		BaseBranch: branch,
	}

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

	var pc policy.Config
	if err := yaml.UnmarshalStrict(c.Content, &pc); err != nil {
		fc.ParseError = err
		return fc
	}
	fc.Config = &pc
	remote := pc.Policy.ApprovalExtends
	if remote == nil {
		return fc
	}

	parent, err := loadConfigWithRetry(ctx, func() (appconfig.Config, error) {
		// The supplied client still belongs to owner, even when the child came from another owner.
		return cf.Loader.LoadRemoteRef(ctx, client, owner, *remote)
	})
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
	fc.Parent, err = decodeParentConfig(parent.Content)
	if err != nil {
		fc.ParseError = fmt.Errorf("failed to parse parent policy %q in %q: %w", parent.Path, parent.Source, err)
	}
	return fc
}

func decodeParentConfig(content []byte) (*policy.Config, error) {
	var fields map[any]any
	if err := yaml.Unmarshal(content, &fields); err != nil {
		return nil, err
	}
	if _, present := fields["remote"]; present {
		return nil, errors.New("parent policy must not be a remote reference")
	}
	var pc policy.Config
	if err := yaml.UnmarshalStrict(content, &pc); err != nil {
		return nil, err
	}
	if pc.Policy.ApprovalExtends != nil {
		return nil, errors.New("parent policy must not extend another policy")
	}
	return &pc, nil
}

func loadConfigWithRetry(ctx context.Context, load func() (appconfig.Config, error)) (appconfig.Config, error) {
	delay := time.Second
	for retries := 0; ; retries++ {
		c, err := load()
		timeout, isTimeout := errors.AsType[interface {
			error
			Timeout() bool
		}](err)
		retryable := (isTimeout && timeout.Timeout()) || isServerError(err)
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
