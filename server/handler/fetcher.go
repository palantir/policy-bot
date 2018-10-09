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
	"fmt"
	"github.com/palantir/go-githubapp/githubapp"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/policy"
)

type FetchedConfig struct {
	Owner  string
	Repo   string
	Ref    string
	Config *policy.Config
	Error  error
}

func (fc FetchedConfig) Missing() bool {
	return fc.Config == nil && fc.Error == nil
}

func (fc FetchedConfig) Valid() bool {
	return fc.Config != nil && fc.Error == nil
}

func (fc FetchedConfig) Invalid() bool {
	return fc.Error != nil
}

func (fc FetchedConfig) String() string {
	return fmt.Sprintf("%s/%s ref=%s", fc.Owner, fc.Repo, fc.Ref)
}

func (fc FetchedConfig) Description() string {
	switch {
	case fc.Missing():
		return fmt.Sprintf("No policy found at ref=%s", fc.Ref)
	case fc.Invalid():
		return fmt.Sprintf("Invalid configuration defined by ref=%s", fc.Ref)
	}
	return fmt.Sprintf("Valid policy found for ref=%s", fc.Ref)
}

type ConfigFetcher struct {
	PolicyPath    string
	Installations githubapp.InstallationClient
	ClientCreator githubapp.ClientCreator
}

// ConfigForPR fetches the policy configuration for a PR. It returns an error
// only if the existence of the policy could not be determined. If the policy
// does not exist or is invalid, the returned error is nil and the appropriate
// fields are set on the FetchedConfig.
func (cf *ConfigFetcher) ConfigForPR(ctx context.Context, client *github.Client, pr *github.PullRequest) (FetchedConfig, error) {
	fc := FetchedConfig{
		Owner: pr.GetBase().GetRepo().GetOwner().GetLogin(),
		Repo:  pr.GetBase().GetRepo().GetName(),
		Ref:   pr.GetBase().GetRef(),
	}

	configBytes, err := cf.fetchConfig(ctx, client, fc.Owner, fc.Repo, fc.Ref)
	if err != nil {
		return fc, err
	}

	if configBytes == nil {
		return fc, nil
	}

	config, err := cf.unmarshalConfig(configBytes)
	if err != nil {
		fc.Error = err
		return fc, nil
	}

	fc.Config = config
	return fc, nil
}

func (cf *ConfigFetcher) fetchConfig(ctx context.Context, client *github.Client, owner, repo, ref string) ([]byte, error) {
	policyBytes, err := cf.fetchConfigContents(ctx, client, owner, repo, ref)
	if err != nil {
		return nil, err
	}

	var remoteConfig policy.RemoteConfig
	if err = yaml.Unmarshal(policyBytes, &remoteConfig); err == nil {
		logger := zerolog.Ctx(ctx)
		logger.Debug().Msgf("fetching remote config from %s", remoteConfig.Remote)

		remoteParts := strings.Split(remoteConfig.Remote, "/")
		if len(remoteParts) != 2 {
			return nil, errors.Errorf("failed to get remote config location of %q", remoteConfig.Remote)
		}

		remoteOwner, remoteRepo := remoteParts[0], remoteParts[1]

		installation, err := cf.Installations.GetByOwner(ctx, remoteOwner)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get installation for remote config %q", remoteConfig.Remote)
		}

		remoteClient, err := cf.ClientCreator.NewInstallationClient(installation.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get installation client for remote config %q", remoteConfig.Remote)
		}

		remotePolicyBytes, err := cf.fetchConfigContents(ctx, remoteClient, remoteOwner, remoteRepo, "")
		if err != nil {
			return nil, err
		}

		return remotePolicyBytes, nil
	}

	return policyBytes, nil
}

// fetchConfigContents returns a nil slice if there is no policy
func (cf *ConfigFetcher) fetchConfigContents(ctx context.Context, client *github.Client, owner, repo, ref string) ([]byte, error) {
	logger := zerolog.Ctx(ctx)
	logger.Debug().Str("path", cf.PolicyPath).Str("ref", ref).Msg("attempting to fetch policy definition")

	opts := &github.RepositoryContentGetOptions{
		Ref: ref,
	}

	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, cf.PolicyPath, opts)
	if err != nil {
		if rerr, ok := err.(*github.ErrorResponse); ok && rerr.Response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to fetch content of %q", cf.PolicyPath)
	}

	// file will be nil if the ref contains a directory at the expected file path
	if file == nil {
		return nil, nil
	}

	content, err := file.GetContent()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode content of %q", cf.PolicyPath)
	}

	return []byte(content), nil
}

func (cf *ConfigFetcher) unmarshalConfig(bytes []byte) (*policy.Config, error) {
	var config policy.Config
	if err := yaml.UnmarshalStrict(bytes, &config); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshall policy")
	}

	return &config, nil
}
