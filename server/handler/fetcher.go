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

	"github.com/google/go-github/v43/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/pull"
	"gopkg.in/yaml.v2"
)

type FetchedConfig struct {
	Config     *policy.Config
	LoadError  error
	ParseError error

	Source string
	Path   string
}

type ConfigFetcher struct {
	Loader *appconfig.Loader
}

func (cf *ConfigFetcher) ConfigForPRBase(ctx context.Context, prctx pull.Context, client *github.Client) FetchedConfig {
	base, _ := prctx.Branches()
	return cf.configForPRBranch(ctx, prctx, client, base)
}

func (cf *ConfigFetcher) ConfigForPRHead(ctx context.Context, prctx pull.Context, client *github.Client) FetchedConfig {
	head := prctx.HeadSHA()
	return cf.configForPRBranch(ctx, prctx, client, head)
}

func (cf *ConfigFetcher) configForPRBranch(ctx context.Context, prctx pull.Context, client *github.Client, ref string) FetchedConfig {
	c, err := cf.Loader.LoadConfig(ctx, client, prctx.RepositoryOwner(), prctx.RepositoryName(), ref)
	fc := FetchedConfig{
		Source: c.Source,
		Path:   c.Path,
	}

	switch {
	case err != nil:
		fc.LoadError = err
		return fc
	case c.IsUndefined():
		return fc
	}

	var pc policy.Config
	if err := yaml.UnmarshalStrict(c.Content, &pc); err != nil {
		fc.ParseError = err
	} else {
		fc.Config = &pc
	}
	return fc
}
