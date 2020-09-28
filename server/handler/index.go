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
	"net/http"

	"github.com/bluekeyes/templatetree"
	"github.com/palantir/go-githubapp/githubapp"

	"github.com/palantir/policy-bot/version"
)

type Index struct {
	Base

	GithubConfig *githubapp.Config
	Templates    templatetree.HTMLTree
}

func (h *Index) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	var data struct {
		AppName    string
		Version    string
		GitHubURL  string
		PolicyPath string
	}

	data.AppName = h.AppName
	data.Version = version.GetVersion()
	data.GitHubURL = h.GithubConfig.WebURL
	data.PolicyPath = h.PullOpts.PolicyPath

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	return h.Templates.ExecuteTemplate(w, "index.html.tmpl", &data)
}
