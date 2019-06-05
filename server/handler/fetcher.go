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
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/pull"
)

type FetchedConfig struct {
	Owner  string
	Repo   string
	Ref    string
	Path   string
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
	PolicyPath string
}

// ConfigForPR fetches the policy configuration for a PR. It returns an error
// only if the existence of the policy could not be determined. If the policy
// does not exist or is invalid, the returned error is nil and the appropriate
// fields are set on the FetchedConfig.
func (cf *ConfigFetcher) ConfigForPR(ctx context.Context, prctx pull.Context, client *github.Client) (FetchedConfig, error) {
	base, _ := prctx.Branches()
	fc := FetchedConfig{
		Owner: prctx.RepositoryOwner(),
		Repo:  prctx.RepositoryName(),
		Ref:   base,
		Path:  cf.PolicyPath,
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
	logger := zerolog.Ctx(ctx)

	configBytes, err := cf.fetchConfigContents(ctx, client, owner, repo, ref, cf.PolicyPath)
	if err != nil {
		return nil, err
	}

	var rawConfig map[string]interface{}
	_ = yaml.Unmarshal(configBytes, &rawConfig)

	if _, isRemote := rawConfig["remote"]; !isRemote {
		logger.Debug().Msgf("Found local policy config in %s/%s@%s", owner, repo, ref)
		return configBytes, nil
	}
	logger.Debug().Msgf("Found reference to remote policy in %s/%s@%s", owner, repo, ref)

	var remoteConfig policy.RemoteConfig
	if err := yaml.UnmarshalStrict(configBytes, &remoteConfig); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal reference to remote policy")
	}

	if remoteConfig.Path == "" {
		remoteConfig.Path = cf.PolicyPath
	}

	remoteParts := strings.Split(remoteConfig.Remote, "/")
	if len(remoteParts) != 2 {
		return nil, errors.Errorf("failed to parse remote config location from %q", remoteConfig.Remote)
	}

	remoteOwner, remoteRepo := remoteParts[0], remoteParts[1]

	remotePolicyBytes, err := cf.fetchConfigContents(ctx, client, remoteOwner, remoteRepo, remoteConfig.Ref, remoteConfig.Path)
	if err != nil {
		return nil, err
	}

	return remotePolicyBytes, nil
}

// fetchConfigContents returns a nil slice if there is no policy
func (cf *ConfigFetcher) fetchConfigContents(ctx context.Context, client *github.Client, owner, repo, ref, path string) ([]byte, error) {
	logger := zerolog.Ctx(ctx)
	logger.Debug().Msgf("attempting to fetch policy definition for %s/%s@%s/%s", owner, repo, ref, path)

	opts := &github.RepositoryContentGetOptions{
		Ref: ref,
	}

	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		rerr, ok := err.(*github.ErrorResponse)
		if ok && rerr.Response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if ok && isTooLargeError(rerr) {
			// GetContents only supports file sizes up to 1 MB, DownloadContents supports files up to 100 MB (with an additional API call)
			reader, err := client.Repositories.DownloadContents(ctx, owner, repo, path, opts)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to download content of %s/%s@%s/%s", owner, repo, ref, path)
			}

			defer func() {
				if cerr := reader.Close(); cerr != nil {
					logger.Error().Err(cerr).Msgf("failed to close reader for %s/%s@%s/%s", owner, repo, ref, path)
				}
			}()
			downloadedContent, readErr := ioutil.ReadAll(reader)
			if readErr != nil {
				return nil, errors.Wrapf(readErr, "failed to read content of %s/%s/@%s/%s", owner, repo, ref, path)
			}
			return downloadedContent, nil
		}
		return nil, errors.Wrapf(err, "failed to fetch content of %s/%s@%s/%s", owner, repo, ref, path)
	}

	// file will be nil if the ref contains a directory at the expected file path
	if file == nil {
		return nil, nil
	}

	content, err := file.GetContent()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode content of %s/%s@%s/%s", owner, repo, ref, path)
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

func isTooLargeError(errorResponse *github.ErrorResponse) bool {
	for _, error := range errorResponse.Errors {
		if error.Code == "too_large" {
			return true
		}
	}
	return false
}
