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

package pull

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	"github.com/google/go-github/v59/github"
	"github.com/google/go-querystring/query"
)

// Based on RepositoriesService.ListTeams from google/go-github
// Modified to include "permissions" map in response struct.
//
// ----
//
// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

func listTeams(ctx context.Context, client *github.Client, owner string, repo string, opts *github.ListOptions) ([]*teamWithPermissions, *github.Response, error) {
	u := fmt.Sprintf("repos/%v/%v/teams", owner, repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var teams []*teamWithPermissions
	resp, err := client.Do(ctx, req, &teams)
	if err != nil {
		return nil, resp, err
	}

	return teams, resp, nil
}

type teamWithPermissions struct {
	github.Team
	Permissions map[string]bool `json:"permissions"`
}

// addOptions adds the parameters in opts as URL query parameters to s. opts
// must be a struct whose fields may contain "url" tags.
func addOptions(s string, opts interface{}) (string, error) {
	v := reflect.ValueOf(opts)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opts)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}
