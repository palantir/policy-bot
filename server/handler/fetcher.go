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

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
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

func (cf *ConfigFetcher) ConfigForRepositoryBranch(ctx context.Context, client *github.Client, owner, repository, branch string) FetchedConfig {

	c, err := cf.Loader.LoadConfig(ctx, client, owner, repository, branch)
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
