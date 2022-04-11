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
	"strconv"
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

func LoadTemplates(c *FilesConfig, basePath string) (templatetree.HTMLTree, error) {
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
		"getPredicateInfo": func(predicateInfo *common.PredicateInfo) []string {
			switch predicateInfo.Name {
			case "Author":
				return []string{predicateInfo.ContributorInfo.Author}
			case "Contributors":
				return predicateInfo.ContributorInfo.Contributors
			case "Target Branch":
				return []string{predicateInfo.BranchInfo.Branch}
			case "Source Branch":
				return []string{predicateInfo.BranchInfo.Branch}
			case "Changed Files":
				if len(predicateInfo.FileInfo.ChangedFiles) <= 4 {
					return predicateInfo.FileInfo.ChangedFiles
				}
				return append(predicateInfo.FileInfo.ChangedFiles[:4], "...")
			case "Modified Lines":
				var modificationStats []string
				AddedLines := predicateInfo.FileInfo.AddedLines
				DeletedLines := predicateInfo.FileInfo.DeletedLines
				TotalModifiedLines := predicateInfo.FileInfo.TotalModifiedLines
				if AddedLines != 0 {
					modificationStats = append(modificationStats, "Addition : "+strconv.FormatInt(AddedLines, 10))
				}
				if DeletedLines != 0 {
					modificationStats = append(modificationStats, "Deletion : "+strconv.FormatInt(DeletedLines, 10))
				}
				if TotalModifiedLines != 0 {
					modificationStats = append(modificationStats, "Total : "+strconv.FormatInt(TotalModifiedLines, 10))
				}
				return modificationStats
			case "Labels":
				return predicateInfo.LabelInfo.PRLabels
			case "Commit Hashes":
				return predicateInfo.CommitInfo.CommitHashes
			case "Commit Hashes and Keys":
				var commitsAndKeys []string
				commitsAndKeys = append(commitsAndKeys, "Commit Hashes : "+strings.Join(predicateInfo.CommitInfo.CommitHashes[:], ", "))
				if len(predicateInfo.CommitInfo.Keys) > 0 {
					commitsAndKeys = append(commitsAndKeys, "Keys : "+strings.Join(predicateInfo.CommitInfo.Keys[:], ", "))
				} else {
					commitsAndKeys = append(commitsAndKeys, "No valid keys because commit hash is not valid.")
				}
				return commitsAndKeys
			case "Commit Hashes and Signers":
				var commitsAndSigners []string
				commitsAndSigners = append(commitsAndSigners, "Commit Hashes : "+strings.Join(predicateInfo.CommitInfo.CommitHashes[:], ", "))
				if len(predicateInfo.CommitInfo.Signers) > 0 {
					commitsAndSigners = append(commitsAndSigners, "Signers : "+strings.Join(predicateInfo.CommitInfo.Signers[:], ", "))
				} else {
					commitsAndSigners = append(commitsAndSigners, "No valid signers because commit hash is not valid.")
				}
				return commitsAndSigners
			case "Status":
				return predicateInfo.StatusInfo.Status
			case "Title":
				return []string{predicateInfo.TitleInfo.PRTitle}
			default:
				return []string{}
			}
		},
		"getPredicateRequirement": func(predicateInfo *common.PredicateInfo) map[string][]string {
			switch predicateInfo.Type {
			case "HasAuthorIn", "OnlyHasContributorsIn", "HasContributorIn":
				membershipInfo := make(map[string][]string)
				if len(predicateInfo.ContributorInfo.Organizations) > 0 {
					membershipInfo["Organizations"] = predicateInfo.ContributorInfo.Organizations
				}
				if len(predicateInfo.ContributorInfo.Teams) > 0 {
					membershipInfo["Teams"] = predicateInfo.ContributorInfo.Teams
				}
				if len(predicateInfo.ContributorInfo.Users) > 0 {
					membershipInfo["Users"] = predicateInfo.ContributorInfo.Users
				}
				return membershipInfo
			case "TargetsBranch", "FromBranch":
				return map[string][]string{"follows patterns": predicateInfo.BranchInfo.Patterns}
			case "ChangedFiles", "OnlyChangedFiles":
				pathInfo := make(map[string][]string)
				if len(predicateInfo.FileInfo.Paths) > 0 {
					pathInfo["are in paths"] = predicateInfo.FileInfo.Paths
				}
				if len(predicateInfo.FileInfo.IgnorePaths) > 0 {
					pathInfo["while ignoring paths"] = predicateInfo.FileInfo.IgnorePaths
				}
				return pathInfo
			case "ModifiedLines":
				modificationInfo := make(map[string][]string)
				if predicateInfo.FileInfo.AdditionLimit != "" {
					modificationInfo["number of lines added is "] = []string{predicateInfo.FileInfo.AdditionLimit}
				}
				if predicateInfo.FileInfo.DeletionLimit != "" {
					modificationInfo["number of lines deleted is"] = []string{predicateInfo.FileInfo.DeletionLimit}
				}
				if predicateInfo.FileInfo.TotalLimit != "" {
					modificationInfo["number of total changes is "] = []string{predicateInfo.FileInfo.TotalLimit}
				}
				return modificationInfo
			case "HasLabels":
				return map[string][]string{"in": predicateInfo.LabelInfo.RequiredLabels}
			case "HasValidSignaturesBy":
				membershipInfo := make(map[string][]string)
				if len(predicateInfo.CommitInfo.Organizations) > 0 {
					membershipInfo["Organizations"] = predicateInfo.CommitInfo.Organizations
				}
				if len(predicateInfo.CommitInfo.Teams) > 0 {
					membershipInfo["Teams"] = predicateInfo.CommitInfo.Teams
				}
				if len(predicateInfo.CommitInfo.Users) > 0 {
					membershipInfo["Users"] = predicateInfo.CommitInfo.Users
				}
				return membershipInfo
			case "HasValidSignaturesByKeys":
				return map[string][]string{"Required Keys": predicateInfo.CommitInfo.RequiredKeys}
			case "Title":
				patternInfo := make(map[string][]string)
				if len(predicateInfo.TitleInfo.MatchPatterns) > 0 {
					patternInfo["matches patterns"] = predicateInfo.TitleInfo.MatchPatterns
				}
				if len(predicateInfo.TitleInfo.NotMatchPatterns) > 0 {
					patternInfo["does not match patterns "] = predicateInfo.TitleInfo.NotMatchPatterns
				}
				return patternInfo
			default:
				return map[string][]string{}
			}
		},
		"isRequiresNotEmpty": func(result *common.Result) bool {
			if len(result.Requires.Organizations) != 0 || len(result.Requires.Teams) != 0 || len(result.Requires.Users) != 0 {
				return true
			}
			return false
		},
		"getRequires": func(result *common.Result) map[string][]Membership {
			membershipInfo := make(map[string][]Membership)
			for _, org := range result.Requires.Organizations {
				if orgs, ok := membershipInfo["Organizations"]; ok {
					membershipInfo["Organizations"] = append(orgs, Membership{Name: org, Link: "https://www.github.com/orgs/" + org})
				} else {
					membershipInfo["Organizations"] = []Membership{{Name: org, Link: "https://www.github.com/orgs/" + org}}
				}
			}
			for _, team := range result.Requires.Teams {
				teamName := strings.Split(team, "/")
				if teams, ok := membershipInfo["Teams"]; ok {
					membershipInfo["Teams"] = append(teams, Membership{Name: team, Link: "https://www.github.com/orgs/" + teamName[0] + "/teams/" + teamName[1] + "/members"})
				} else {
					membershipInfo["Teams"] = []Membership{{Name: team, Link: "https://www.github.com/orgs/" + teamName[0] + "/teams/" + teamName[1] + "/members"}}
				}
			}
			for _, user := range result.Requires.Users {
				if users, ok := membershipInfo["Users"]; ok {
					membershipInfo["Users"] = append(users, Membership{Name: user, Link: "https://www.github.com/" + user})
				} else {
					membershipInfo["Users"] = []Membership{{Name: user, Link: "https://www.github.com/" + user}}
				}
			}
			return membershipInfo
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
