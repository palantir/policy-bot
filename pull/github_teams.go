// Copyright 2020 Palantir Technologies, Inc.
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

// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pull

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	"github.com/google/go-github/v31/github"
	"github.com/google/go-querystring/query"
)

// TODO(bkeyes): this is a temporary workarount to add functions for APIs that
// are still supported by GitHub but were removed by google/go-github in
// v29.0.3. The replacement methods have not shipped in a GitHub Enterprise
// release yet, so we can't start using them.

const mediaTypeNestedTeamsPreview = "application/vnd.github.hellcat-preview+json"

func GetTeamMembership(ctx context.Context, client *github.Client, team int64, user string) (*github.Membership, *github.Response, error) {
	u := fmt.Sprintf("teams/%v/memberships/%v", team, user)
	req, err := client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeNestedTeamsPreview)

	t := new(github.Membership)
	resp, err := client.Do(ctx, req, t)
	if err != nil {
		return nil, resp, err
	}

	return t, resp, nil
}

func ListTeamMembers(ctx context.Context, client *github.Client, team int64, opt *github.TeamListTeamMembersOptions) ([]*github.User, *github.Response, error) {
	u := fmt.Sprintf("teams/%v/members", team)
	u, err := addOptions(u, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeNestedTeamsPreview)

	var members []*github.User
	resp, err := client.Do(ctx, req, &members)
	if err != nil {
		return nil, resp, err
	}

	return members, resp, nil
}

// addOptions adds the parameters in opt as URL query parameters to s. opt
// must be a struct whose fields may contain "url" tags.
func addOptions(s string, opt interface{}) (string, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}
