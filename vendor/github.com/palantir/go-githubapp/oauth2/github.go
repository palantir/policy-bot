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

package oauth2

import (
	"strings"

	"github.com/palantir/go-githubapp/githubapp"
	"golang.org/x/oauth2"
)

const (
	DefaultRoute = "/api/github/auth"
)

func GetConfig(c githubapp.Config, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.OAuth.ClientID,
		ClientSecret: c.OAuth.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  joinURL(c.WebURL, "/login/oauth/authorize"),
			TokenURL: joinURL(c.WebURL, "/login/oauth/access_token"),
		},
		Scopes: scopes,
	}
}

func joinURL(base, path string) string {
	return strings.TrimSuffix(base, "/") + path
}
