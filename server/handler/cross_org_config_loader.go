// Copyright 2026 Palantir Technologies, Inc.
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
	"strings"

	"github.com/google/go-github/v90/github"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
)

// CrossOrgConfigLoader loads repository configuration, resolving "remote"
// references (see the "Remote Policy Configuration" section of the README)
// to a repository in a different GitHub organization by looking up that
// organization's own app installation.
//
// Without this, a "remote: org/repo" reference only works if org is the
// same as the requesting repository's org, because the underlying
// appconfig.Loader reuses the requesting repository's installation client
// for the remote fetch.
type CrossOrgConfigLoader struct {
	inner ConfigLoader
	paths []string

	clientCreator githubapp.ClientCreator
	installations githubapp.InstallationsService

	// newRemoteLoader builds the ConfigLoader used to fetch the resolved
	// remote file. It is a field, rather than a direct call to
	// appconfig.NewLoader, so tests can substitute a fake loader and avoid
	// making real GitHub content-fetch requests.
	newRemoteLoader func(paths []string) ConfigLoader
}

// NewCrossOrgConfigLoader creates a CrossOrgConfigLoader. paths and opts
// configure local configuration loading exactly like appconfig.NewLoader.
// Any appconfig.WithRemoteRefParser option passed in opts is overridden:
// CrossOrgConfigLoader always disables the inner loader's remote-following
// and handles "remote:" references itself.
func NewCrossOrgConfigLoader(paths []string, cc githubapp.ClientCreator, installations githubapp.InstallationsService, opts ...appconfig.Option) *CrossOrgConfigLoader {
	innerOpts := append(append([]appconfig.Option{}, opts...), appconfig.WithRemoteRefParser(nil))

	return &CrossOrgConfigLoader{
		inner:         appconfig.NewLoader(paths, innerOpts...),
		paths:         paths,
		clientCreator: cc,
		installations: installations,
		newRemoteLoader: func(paths []string) ConfigLoader {
			return appconfig.NewLoader(paths, appconfig.WithRemoteRefParser(nil), appconfig.WithOwnerDefault("", nil))
		},
	}
}

// LoadConfig implements ConfigLoader.
func (l *CrossOrgConfigLoader) LoadConfig(ctx context.Context, client *github.Client, owner, repo, ref string) (appconfig.Config, error) {
	cfg, err := l.inner.LoadConfig(ctx, client, owner, repo, ref)
	if err != nil || cfg.IsUndefined() {
		return cfg, err
	}

	remote, err := appconfig.YAMLRemoteRefParser(cfg.Path, cfg.Content)
	if err != nil {
		return cfg, err
	}
	if remote == nil {
		// Not a remote reference: a normal local (or org-default) policy file.
		return cfg, nil
	}

	remoteOwner, remoteRepo, err := remote.SplitRemote()
	if err != nil {
		return cfg, err
	}

	remoteClient := client
	if !strings.EqualFold(remoteOwner, owner) {
		remoteClient, err = l.resolveClient(ctx, client, remoteOwner)
		if err != nil {
			return cfg, fmt.Errorf("policy-bot is not installed on organization %q, cannot load remote policy %q: %w", remoteOwner, remote.Remote, err)
		}
	}

	path := remote.Path
	if path == "" && len(l.paths) > 0 {
		path = l.paths[0]
	}

	remoteRef := remote.Ref
	if remoteRef == "" {
		r, _, err := remoteClient.Repositories.Get(ctx, remoteOwner, remoteRepo)
		if err != nil {
			return cfg, fmt.Errorf("failed to get remote repository %s/%s: %w", remoteOwner, remoteRepo, err)
		}
		remoteRef = r.GetDefaultBranch()
	}

	remoteLoader := l.newRemoteLoader([]string{path})
	remoteCfg, err := remoteLoader.LoadConfig(ctx, remoteClient, remoteOwner, remoteRepo, remoteRef)
	if err != nil {
		return remoteCfg, err
	}
	if remoteCfg.IsUndefined() {
		return remoteCfg, fmt.Errorf("invalid remote reference: file does not exist")
	}

	remoteCfg.Source = fmt.Sprintf("%s/%s@%s", remoteOwner, remoteRepo, remoteRef)
	remoteCfg.Path = path
	remoteCfg.IsRemote = true
	return remoteCfg, nil
}

// resolveClient returns a *github.Client authenticated as the app's
// installation on orgName, using client (the requesting repository's
// client) to resolve the organization's canonical login first.
func (l *CrossOrgConfigLoader) resolveClient(ctx context.Context, client *github.Client, orgName string) (*github.Client, error) {
	org, _, err := client.Organizations.Get(ctx, orgName)
	if err != nil {
		return nil, err
	}

	installation, err := l.installations.GetByOwner(ctx, org.GetLogin())
	if err != nil {
		return nil, err
	}

	return l.clientCreator.NewInstallationClient(installation.ID)
}
