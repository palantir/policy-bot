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
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/templatetree"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"goji.io/pat"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type Details struct {
	Base
	Sessions  *scs.Manager
	Templates templatetree.HTMLTree
}

func (h *Details) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	owner := pat.Param(r, "owner")
	repo := pat.Param(r, "repo")

	number, err := strconv.Atoi(pat.Param(r, "number"))
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid pull request number: %v", err), http.StatusBadRequest)
		return nil
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		return err
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		return errors.Wrap(err, "failed to create github client")
	}

	v4client, err := h.ClientCreator.NewInstallationV4Client(installation.ID)
	if err != nil {
		return errors.Wrap(err, "failed to create github client")
	}

	sess := h.Sessions.Load(r)
	user, err := sess.GetString(SessionKeyUsername)
	if err != nil {
		return errors.Wrap(err, "failed to read sessions")
	}

	level, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, user)
	if err != nil {
		if isNotFound(err) {
			http.Error(w, fmt.Sprintf("not found: %s/%s#%d", owner, repo, number), http.StatusNotFound)
			return nil
		}
		return errors.Wrap(err, "failed to get user permission level")
	}

	// if the user does not have permission, pretend the repo/PR doesn't exist
	if level.GetPermission() == "none" {
		http.Error(w, fmt.Sprintf("not found: %s/%s#%d", owner, repo, number), http.StatusNotFound)
		return nil
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			http.Error(w, fmt.Sprintf("not found: %s/%s#%d", owner, repo, number), http.StatusNotFound)
			return nil
		}
		return errors.Wrap(err, "failed to get pull request")
	}

	ctx, _ = h.PreparePRContext(ctx, installation.ID, pr)

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, owner, h.Installations, h.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, pull.Locator{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		Value:  pr,
	})
	if err != nil {
		return err
	}

	var data struct {
		Error       error
		Result      *common.Result
		PullRequest *github.PullRequest
		User        string
		PolicyURL   string
	}

	data.PullRequest = pr
	data.User = user

	config, err := h.ConfigFetcher.ConfigForPR(ctx, prctx, client)
	data.PolicyURL = getPolicyURL(pr, config)

	if err != nil {
		data.Error = errors.WithMessage(err, fmt.Sprintf("Failed to fetch configuration at ref=%s", config.Ref))
		return h.render(w, data)
	}

	if config.Missing() {
		data.Error = errors.New(config.Description())
		return h.render(w, data)
	}

	if config.Invalid() {
		data.Error = errors.WithMessage(config.Error, config.Description())
		return h.render(w, data)
	}

	evaluator, err := policy.ParsePolicy(config.Config)
	if err != nil {
		data.Error = errors.WithMessage(err, fmt.Sprintf("invalid policy at ref \"%s\"", config.Ref))
		return h.render(w, data)
	}

	result := evaluator.Evaluate(ctx, prctx)
	data.Result = &result

	return h.render(w, data)
}

func (h *Details) render(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	return h.Templates.ExecuteTemplate(w, "details.html.tmpl", data)
}

func getPolicyURL(pr *github.PullRequest, config FetchedConfig) string {
	base := pr.GetBase().GetRepo().GetHTMLURL()
	if u, _ := url.Parse(base); u != nil {
		u.Path = path.Join(u.Path, "blob", pr.GetBase().GetRef(), config.Path)
		return u.String()
	}
	return base
}

func isNotFound(err error) bool {
	rerr, ok := err.(*github.ErrorResponse)
	return ok && rerr.Response.StatusCode == http.StatusNotFound
}
