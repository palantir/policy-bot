// Copyright 2023 Palantir Technologies, Inc.
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
	"slices"
	"strings"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type DetailsReviewers struct {
	Details
}

type ReviewerInfo struct {
	Name      string // Username/login
	Username  string // Secondary identifier (usually same as Name)
	Link      string
	AvatarURL string
}

type ReviewerGroup struct {
	Name      string         // Group display name (team slug, org name, or "Other")
	Reviewers []ReviewerInfo // Reviewers in this group
}

type DetailsReviewersData struct {
	PullRequest *github.PullRequest
	Groups      []ReviewerGroup
	Incomplete  bool
}

// reviewerCollector collects reviewers grouped by source.
type reviewerCollector struct {
	groups map[string]*reviewerGroupData
	seen   map[string]struct{} // track all seen usernames to avoid duplicates
}

type reviewerGroupData struct {
	label     string
	reviewers []ReviewerInfo
}

func newReviewerCollector() *reviewerCollector {
	return &reviewerCollector{
		groups: make(map[string]*reviewerGroupData),
		seen:   make(map[string]struct{}),
	}
}

func (c *reviewerCollector) add(groupKey, groupLabel, name, link, avatarURL string) {
	if _, ok := c.seen[name]; ok {
		return
	}
	c.seen[name] = struct{}{}

	group := c.groups[groupKey]
	if group == nil {
		group = &reviewerGroupData{label: groupLabel}
		c.groups[groupKey] = group
	}

	group.reviewers = append(group.reviewers, ReviewerInfo{
		Name:      name,
		Username:  name,
		Link:      link,
		AvatarURL: avatarURL,
	})
}

func (c *reviewerCollector) toGroups() []ReviewerGroup {
	// Collect and sort group keys by label, then key for stability.
	groupKeys := make([]string, 0, len(c.groups))
	for key := range c.groups {
		groupKeys = append(groupKeys, key)
	}
	slices.SortFunc(groupKeys, func(a, b string) int {
		if c.groups[a].label == c.groups[b].label {
			return strings.Compare(a, b)
		}
		return strings.Compare(c.groups[a].label, c.groups[b].label)
	})

	// Build sorted groups
	result := make([]ReviewerGroup, 0, len(c.groups))
	for _, groupKey := range groupKeys {
		group := c.groups[groupKey]
		reviewers := group.reviewers
		slices.SortFunc(reviewers, func(a, b ReviewerInfo) int {
			return strings.Compare(a.Name, b.Name)
		})
		result = append(result, ReviewerGroup{
			Name:      group.label,
			Reviewers: reviewers,
		})
	}
	return result
}

func (h *DetailsReviewers) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	if !h.PullOpts.ExpandRequiredReviewers {
		return h.renderEmptyReviewers(w, r, nil)
	}

	state := h.getStateIfAllowed(w, r)
	if state == nil {
		return nil
	}

	evalctx := state.EvalContext
	prctx := evalctx.PullContext
	logger := state.Logger

	config := evalctx.Config
	switch {
	case config.LoadError != nil:
		logger.Warn().Err(config.LoadError).Msgf("Error loading policy from %s, reviewers will be incomplete", config.Source)
		return h.renderReviewers(w, r, DetailsReviewersData{PullRequest: state.PullRequest, Incomplete: true})

	case config.Config == nil:
		// The repository either has no policy or the policy was invalid. This
		// should never happen in normal use, because the expected way to hit
		// this endpoint requires viewing a valid details page first. Hitting
		// this case means something unexpected is hitting the API directly.
		return h.renderEmptyReviewers(w, r, state.PullRequest)
	}

	ruleName := r.URL.Query().Get("rule")
	requires := findRuleRequires(config.Config, ruleName)

	if requires == nil || requires.Count == 0 || requires.Actors.IsZero() {
		// If the rule does not exist, it does not require approval, or it has
		// no actors specified, there's no need to list reviewers
		return h.renderEmptyReviewers(w, r, state.PullRequest)
	}

	collector := newReviewerCollector()
	incomplete := false

	githubURL := defaultGitHubURL(evalctx.PublicURL)

	// Add direct users (group: "Users")
	for _, user := range requires.Actors.Users {
		collector.add("Users", "Users", user, githubURL+"/"+user, "")
	}

	// Add organization members
	for _, org := range requires.Actors.Organizations {
		members, err := prctx.OrganizationMembers(org)
		if err != nil {
			logger.Warn().Err(err).Str("organization", org).Msg("Error listing organization members, reviewers will be incomplete")
			incomplete = true
		} else {
			for _, m := range members {
				collector.add(org, org, m, githubURL+"/"+m, "")
			}
		}
	}

	// Add team members with details (avatars, display names)
	for _, team := range requires.Actors.Teams {
		groupKey, groupLabel := teamGroupKeyLabel(team)
		members, err := prctx.TeamMembersWithDetails(team)
		if err != nil {
			logger.Warn().Err(err).Str("team", team).Msg("Error listing team members, reviewers will be incomplete")
			incomplete = true
		} else {
			for _, m := range members {
				collector.add(groupKey, groupLabel, m.Login, m.HTMLURL, m.AvatarURL)
			}
		}
	}

	perms := requires.Actors.GetPermissions()
	if len(perms) > 0 {
		minPerm := slices.Min(perms)
		userCollaborators, err := prctx.RepositoryCollaborators(minPerm)
		if err != nil {
			logger.Warn().Err(err).Msg("Error listing user collaborators, reviewers will be incomplete")
			incomplete = true
		} else {
			for _, user := range userCollaborators {
				if userHasReviewerPermission(user, perms) {
					collector.add("Collaborators", "Collaborators", user.Name, githubURL+"/"+user.Name, "")
				}
			}
		}
	}

	// Add codeowners (both direct users and team members)
	if requires.Actors.Codeowners {
		co, err := prctx.Codeowners()
		if err != nil {
			logger.Warn().Err(err).Msg("Error loading codeowners, reviewers will be incomplete")
			incomplete = true
		} else if co != nil {
			for _, owner := range co.AllOwners() {
				ownerType, name := pull.ParseCodeowner(owner)
				switch ownerType {
				case "user":
					collector.add("Codeowners", "Codeowners", name, githubURL+"/"+name, "")
				case "team":
					groupKey, groupLabel := teamGroupKeyLabel(name)
					members, err := prctx.TeamMembersWithDetails(name)
					if err != nil {
						logger.Warn().Err(err).Str("team", name).Msg("Error listing codeowner team members, reviewers will be incomplete")
						incomplete = true
					} else {
						for _, m := range members {
							collector.add(groupKey, groupLabel, m.Login, m.HTMLURL, m.AvatarURL)
						}
					}
				}
			}
		}
	}

	return h.renderReviewers(w, r, DetailsReviewersData{
		PullRequest: state.PullRequest,
		Groups:      collector.toGroups(),
		Incomplete:  incomplete,
	})
}

func (h *DetailsReviewers) renderEmptyReviewers(w http.ResponseWriter, r *http.Request, pr *github.PullRequest) error {
	return h.renderReviewers(w, r, DetailsReviewersData{PullRequest: pr})
}

func (h *DetailsReviewers) renderReviewers(w http.ResponseWriter, r *http.Request, data DetailsReviewersData) error {
	tmpl, ok := h.Templates["details_reviewers.html.tmpl"]
	if !ok {
		return errors.New("no template named \"details_reviewers.html.tmpl\"")
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	if r.Header.Get("HX-Request") == "true" {
		return tmpl.ExecuteTemplate(w, "body", data)
	}
	return tmpl.Execute(w, data)
}

func userHasReviewerPermission(user *pull.Collaborator, perms []pull.Permission) bool {
	for _, p := range user.Permissions {
		if p.ViaRepo && slices.Contains(perms, p.Permission) {
			return true
		}
	}
	return false
}

func findRuleRequires(config *policy.Config, ruleName string) *approval.Requires {
	for _, rule := range config.ApprovalRules {
		if rule.Name == ruleName {
			return &rule.Requires
		}
	}
	return nil
}

func defaultGitHubURL(publicURL string) string {
	if publicURL == "" {
		return "https://github.com"
	}
	return publicURL
}

// teamGroupKeyLabel returns the key (full team name) and label (team slug only)
// for grouping reviewers by team. For "org/team-slug", returns ("org/team-slug", "team-slug").
func teamGroupKeyLabel(team string) (key, label string) {
	if idx := strings.LastIndex(team, "/"); idx != -1 {
		return team, team[idx+1:]
	}
	return team, team
}
