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

/*

Not sure this is valuable yet, but leaving this option function as a starting
point for a future implementation. See https://github.com/palantir/policy-bot/issues/111
for some explanation of why this is desired.

In the Loader implementation, if a ClientCreator and InstallationsService are
set, the loadRemoteConfig method would use them to create a new client if the
remote owner does not equal the starting owner.

// WithPrivateRemotes enables loading remote configuration from private
// repositories in different organizations. By default, only public
// repositories can be remote targets.
func WithPrivateRemotes(cc githubapp.ClientCreator, installs githubapp.InstallationsService) Option {
	return func(ld *Loader) {}
}
*/
