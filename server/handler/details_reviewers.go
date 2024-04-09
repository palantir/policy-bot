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

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type DetailsReviewers struct {
	Details
}

type DetailsReviewersData struct {
	Reviewers  []string
	Incomplete bool
}

func (h *DetailsReviewers) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	if !h.PullOpts.ExpandRequiredReviewers {
		return h.renderEmptyReviewers(w, r)
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
		return h.renderReviewers(w, r, DetailsReviewersData{Incomplete: true})

	case config.Config == nil:
		// The repository either has no policy or the policy was invalid. This
		// should never happen in normal use, because the expected way to hit
		// this endpoint requires viewing a valid details page first. Hitting
		// this case means something unexpected is hitting the API directly.
		return h.renderEmptyReviewers(w, r)
	}

	ruleName := r.URL.Query().Get("rule")
	requires := findRuleRequires(config.Config, ruleName)

	if requires == nil || requires.Count == 0 || requires.Actors.IsEmpty() {
		// If the rule does not exist, it does not require approval, or it has
		// no actors specified, there's no need to list reviewers
		return h.renderEmptyReviewers(w, r)
	}

	var reviewers []string
	var incomplete bool

	// Add direct users
	reviewers = append(reviewers, requires.Actors.Users...)

	// Add organization members
	for _, org := range requires.Actors.Organizations {
		members, err := prctx.OrganizationMembers(org)
		if err != nil {
			logger.Warn().Err(err).Str("organization", org).Msg("Error listing organization members, reviewers will be incomplete")
			incomplete = true
		} else {
			reviewers = append(reviewers, members...)
		}
	}

	// Add team members
	for _, team := range requires.Actors.Teams {
		members, err := prctx.TeamMembers(team)
		if err != nil {
			logger.Warn().Err(err).Str("team", team).Msg("Error listing team members, reviewers will be incomplete")
			incomplete = true
		} else {
			reviewers = append(reviewers, members...)
		}
	}

	// Add reviewers with permissions
	perms := requires.Actors.GetPermissions()
	if len(perms) > 0 {
		userCollaborators, err := prctx.RepositoryCollaborators()
		if err != nil {
			logger.Warn().Err(err).Msg("Error listing user collaborators, reviewers will be incomplete")
			incomplete = true
		} else {
			for _, user := range userCollaborators {
				if userHasReviewerPermission(user, perms) {
					reviewers = append(reviewers, user.Name)
				}
			}
		}
	}

	// Order the reviewers and remove any duplicates
	slices.Sort(reviewers)
	reviewers = slices.Compact(reviewers)

	return h.renderReviewers(w, r, DetailsReviewersData{
		Reviewers:  reviewers,
		Incomplete: incomplete,
	})
}

func (h *DetailsReviewers) renderEmptyReviewers(w http.ResponseWriter, r *http.Request) error {
	return h.renderReviewers(w, r, DetailsReviewersData{})
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
