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
	"errors"

	"gopkg.in/yaml.v2"
)

// YAMLRemoteRefParser parses b as a YAML-encoded RemoteRef. It assumes all
// parsing errors mean the content is not a RemoteRef.
func YAMLRemoteRefParser(path string, b []byte) (*RemoteRef, error) {
	var ref RemoteRef
	if err := yaml.UnmarshalStrict(b, &ref); err != nil {
		// assume errors mean this isn't a remote config
		return nil, nil
	}

	if ref.Remote == "" {
		return nil, errors.New("invalid remote reference: empty \"remote\" field")
	}
	if ref.Path == "" {
		return nil, errors.New("invalid remote references: empty \"path\" field")
	}
	return &ref, nil
}
