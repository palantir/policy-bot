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

func LoadTemplates(c *FilesConfig) (templatetree.HTMLTree, error) {
	root := template.New("root").Funcs(template.FuncMap{
		"titlecase": strings.Title,
		"sortByStatus": func(results []*common.Result) []*common.Result {
			r := make([]*common.Result, len(results))
			copy(r, results)

			sort.SliceStable(r, func(i, j int) bool {
				return r[i].Status > r[j].Status
			})

			return r
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
