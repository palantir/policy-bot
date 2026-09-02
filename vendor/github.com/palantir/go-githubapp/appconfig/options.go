// Copyright 2021 Palantir Technologies, Inc.
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

package appconfig

import (
	"github.com/palantir/go-githubapp/githubapp"
)

type Option func(*Loader)

// WithRemoteRefParser sets the parser for encoded RemoteRefs. The default
// parser uses YAML. Set a nil parser to disable remote references.
func WithRemoteRefParser(parser RemoteRefParser) Option {
	return func(ld *Loader) {
		ld.parser = parser
	}
}

// WithOwnerDefault sets the owner repository and paths to check when a
// repository does not define its own configuration. By default, the repository
// name is ".github" and the paths are those passed to the loader with the
// ".github/" prefix removed. Set an empty repository name to disable
// owner defaults.
func WithOwnerDefault(name string, paths []string) Option {
	return func(ld *Loader) {
		ld.defaultRepo = name
		ld.defaultPaths = paths
	}
}

// WithPrivateRemotes enables loading remote configuration from private
// repositories owned by a different user or organization. It uses the app's
// installation on the remote repository to fetch the referenced file. If the
// app is not installed on that repository, or an installation client cannot be created,
// the loader logs the failure and falls back to the original client so public
// remote repositories remain supported.
//
// WARNING: Enabling this option can expose configuration file content and
// repository existence to users who otherwise may not be able to access them.
// Enable it only when the app is installed on GitHub organizations where all
// users are trusted, or when the caller otherwise prevents unintentional
// information disclosure.
//
// This loader does not cache installation lookups or clients. Callers that
// load configuration frequently should pass caching implementations, such as
// githubapp.NewCachingInstallationsService and githubapp.NewCachingClientCreator.
func WithPrivateRemotes(clientCreator githubapp.ClientCreator, installations githubapp.InstallationsService) Option {
	return func(ld *Loader) {
		ld.clientCreator = clientCreator
		ld.installations = installations
	}
}
