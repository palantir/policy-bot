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
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluekeyes/templatetree"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/pkg/errors"
)

const (
	DefaultTemplatesDir = "templates"
	DefaultStaticDir    = "static"

	ManifestFile = "manifest.json"
)

type FilesConfig struct {
	Static    string `yaml:"static"`
	Templates string `yaml:"templates"`
}

type Membership struct {
	Name string
	Link string
}

func LoadTemplates(c *FilesConfig, basePath string, githubURL string) (templatetree.Tree[*template.Template], error) {
	if basePath == "" {
		basePath = "/"
	}

	tmplDir := c.Templates
	if tmplDir == "" {
		tmplDir = DefaultTemplatesDir
	}

	staticDir := c.Static
	if staticDir == "" {
		staticDir = DefaultStaticDir
	}

	githubURL = strings.TrimSuffix(githubURL, "/")

	manifest, err := loadManifest(staticDir)
	if err != nil {
		return nil, err
	}

	return templatetree.Parse(tmplDir, "*.html.tmpl", func(name string) templatetree.Template[*template.Template] {
		return template.New(name).Funcs(template.FuncMap{
			"args": func(args ...any) []any {
				return args
			},
			"urlencode": func(val string) string {
				return url.QueryEscape(val)
			},
			"githubURL": func(parts ...string) string {
				if len(parts) == 0 {
					return githubURL
				}
				return githubURL + "/" + path.Join(parts...)
			},
			"resource": func(r string) string {
				if hashed, ok := manifest[r]; ok {
					r = hashed
				}
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
			"hasActors": func(requires *common.Requires) bool {
				return len(requires.Actors.Users) > 0 || len(requires.Actors.Teams) > 0 || len(requires.Actors.Organizations) > 0
			},
			"getMethods": func(results *common.Result) map[string][]string {
				return getMethods(results)
			},
			"getActors": func(results *common.Result) map[string][]Membership {
				return getActors(results, strings.TrimSuffix(githubURL, "/"))
			},
			"hasActorsPermissions": func(requires *common.Requires) bool {
				return len(requires.Actors.GetPermissions()) > 0
			},
			"getPermissions": func(results *common.Result) []string {
				return getPermissions(results)
			},
			"nextStatus": func(i int, results []*common.Result) string {
				if i < len(results)-1 {
					return results[i+1].Status.String()
				}
				return ""
			},
		})
	})
}

func loadManifest(dir string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to read manifest file")
	}

	var manifest map[string]string
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal manifest")
	}
	return manifest, nil
}

func Static(prefix string, c *FilesConfig) http.Handler {
	dir := c.Static
	if dir == "" {
		dir = DefaultStaticDir
	}

	manifest, _ := loadManifest(dir)

	h := http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
	if manifest == nil {
		return h
	}

	// If a manifest exists, it implies we're using hashed assets. In this
	// case, instruct browsers to cache them for 1yr to improve load time
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "public, max-age=31536000")
		h.ServeHTTP(w, r)
	})
}

func getMethods(result *common.Result) map[string][]string {
	const (
		commentKey        = "Comments containing"
		commentPatternKey = "Comments matching patterns"
		bodyPatternKey    = "The pull request body matching patterns"
		reviewKey         = "GitHub reviews with status"
	)

	patternInfo := make(map[string][]string)
	for _, comment := range result.Methods.Comments {
		patternInfo[commentKey] = append(patternInfo[commentKey], comment)
	}
	for _, commentPattern := range result.Methods.CommentPatterns {
		patternInfo[commentPatternKey] = append(patternInfo[commentPatternKey], commentPattern.String())
	}
	for _, bodyPattern := range result.Methods.BodyPatterns {
		patternInfo[bodyPatternKey] = append(patternInfo[bodyPatternKey], bodyPattern.String())
	}
	if result.Methods.GithubReview != nil && *result.Methods.GithubReview {
		reviewPatternKey := reviewKey + fmt.Sprintf(" %s matching patterns", result.Methods.GithubReviewState)
		if len(result.Methods.GithubReviewCommentPatterns) > 0 {
			for _, githubReviewCommentPattern := range result.Methods.GithubReviewCommentPatterns {
				patternInfo[reviewPatternKey] = append(patternInfo[reviewPatternKey], githubReviewCommentPattern.String())
			}
		} else {
			patternInfo[reviewKey] = append(patternInfo[reviewKey], string(result.Methods.GithubReviewState))
		}
	}
	return patternInfo
}

func getActors(result *common.Result, githubURL string) map[string][]Membership {
	const (
		orgKey  = "Members of the organizations"
		teamKey = "Members of the teams"
		userKey = "Users"
	)

	membershipInfo := make(map[string][]Membership)
	for _, org := range result.Requires.Actors.Organizations {
		membershipInfo[orgKey] = append(membershipInfo[orgKey], Membership{Name: org, Link: githubURL + "/orgs/" + org + "/people"})
	}
	for _, team := range result.Requires.Actors.Teams {
		teamName := strings.Split(team, "/")
		membershipInfo[teamKey] = append(membershipInfo[teamKey], Membership{Name: team, Link: githubURL + "/orgs/" + teamName[0] + "/teams/" + teamName[1] + "/members"})

	}
	for _, user := range result.Requires.Actors.Users {
		membershipInfo[userKey] = append(membershipInfo[userKey], Membership{Name: user, Link: githubURL + "/" + user})
	}
	return membershipInfo
}

func getPermissions(result *common.Result) []string {
	perms := result.Requires.Actors.GetPermissions()
	permStrings := make([]string, 0, len(perms))
	for _, perm := range perms {
		permStrings = append(permStrings, perm.String())
	}
	return permStrings
}
