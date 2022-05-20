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
	"html/template"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/bluekeyes/templatetree"
	"github.com/palantir/policy-bot/policy/common"
)

const (
	DefaultTemplatesDir = "templates"
	DefaultStaticDir    = "static"
)

type FilesConfig struct {
	Static    string `yaml:"static"`
	Templates string `yaml:"templates"`
}

type Membership struct {
	Name string
	Link string
}

func LoadTemplates(c *FilesConfig, basePath string, githubURL string) (templatetree.HTMLTree, error) {
	if basePath == "" {
		basePath = "/"
	}

	root := template.New("root").Funcs(template.FuncMap{
		"resource": func(r string) string {
			return path.Join(basePath, "static", r)
		},
		"titlecase": strings.Title,
		"sortByStatus": func(results []*common.Result) []*common.Result {
			r := make([]*common.Result, len(results))
			copy(r, results)

			sort.SliceStable(r, func(i, j int) bool {
				return r[i].Status > r[j].Status
			})

			return r
		},
		"hasRequires": func(requires common.Actors) bool {
			return len(requires.Users) > 0 || len(requires.Teams) > 0 || len(requires.Organizations) > 0
		},
		"getRequires": func(results *common.Result) map[string][]Membership {
			return getRequires(results, strings.TrimSuffix(githubURL, "/"))
		},
		"hasRequiresPermissions": func(requires common.Actors) bool {
			return len(requires.GetPermissions()) > 0
		},
		"getPermissions": func(results *common.Result) []string {
			return getPermissions(results)
		},
	})

	dir := c.Templates
	if dir == "" {
		dir = DefaultTemplatesDir
	}

	return templatetree.LoadHTML(dir, "*.html.tmpl", root)
}

func Static(prefix string, c *FilesConfig) http.Handler {
	dir := c.Static
	if dir == "" {
		dir = DefaultStaticDir
	}

	return http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
}

func getRequires(result *common.Result, githubURL string) map[string][]Membership {
	membershipInfo := make(map[string][]Membership)
	for _, org := range result.Requires.Organizations {
		membershipInfo["Organizations"] = append(membershipInfo["Organizations"], Membership{Name: org, Link: githubURL + "/orgs/" + org + "/people"})
	}
	for _, team := range result.Requires.Teams {
		teamName := strings.Split(team, "/")
		membershipInfo["Teams"] = append(membershipInfo["Teams"], Membership{Name: team, Link: githubURL + "/orgs/" + teamName[0] + "/teams/" + teamName[1] + "/members"})

	}
	for _, user := range result.Requires.Users {
		membershipInfo["Users"] = append(membershipInfo["Users"], Membership{Name: user, Link: githubURL + "/" + user})
	}
	return membershipInfo
}

func getPermissions(result *common.Result) []string {
	perms := result.Requires.GetPermissions()
	permStrings := make([]string, 0, len(perms))
	for _, perm := range perms {
		permStrings = append(permStrings, perm.String())
	}
	return permStrings
}
