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
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/hatpear"
	"github.com/bluekeyes/templatetree"
	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"goji.io/pat"
)

type Details struct {
	Base
	Sessions  *scs.Manager
	Templates templatetree.Tree[*template.Template]
}

// DetailsState combines fields that the Details handler and related
// sub-handlers need to process requests
type DetailsState struct {
	Ctx         context.Context
	Logger      zerolog.Logger
	EvalContext *EvalContext

	Username    string
	PullRequest *github.PullRequest
}

func (h *Details) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	state := h.getStateIfAllowed(w, r)
	if state == nil {
		return nil
	}

	ctx := state.Ctx
	evalCtx := state.EvalContext

	var data struct {
		BasePath  string
		User      string
		PolicyURL string

		ExpandRequiredReviewers bool

		Error            error
		IsTemporaryError bool

		PullRequest *github.PullRequest
		Result      *common.Result
	}

	data.BasePath = getBasePath(h.BaseConfig.PublicURL)
	data.User = state.Username
	data.PolicyURL = getPolicyURL(state.PullRequest, evalCtx.Config)
	data.ExpandRequiredReviewers = h.PullOpts.ExpandRequiredReviewers
	data.PullRequest = state.PullRequest

	evaluator, err := evalCtx.ParseConfig(ctx, common.TriggerAll)
	if err != nil {
		data.Error = err
		return h.render(w, data)
	}
	if evaluator == nil {
		data.Error = errors.Errorf("Invalid policy at %s: %s", evalCtx.Config.Source, evalCtx.Config.Path)
		return h.render(w, data)
	}

	result, err := evalCtx.EvaluatePolicy(ctx, evaluator)
	data.Result = &result

	if err != nil {
		if _, ok := errors.Cause(err).(*pull.TemporaryError); ok {
			data.IsTemporaryError = true
		}
		data.Error = err
	}

	// Intentionally skip evalCtx.RunPostEvaluateActions() for details
	// evaluations to minimize side-effects when viewing policy status. These
	// actions are best-effort, so if they were missed by normal event
	// handling, we don't _need_ to retry them here.

	return h.render(w, data)
}

// getStateIfAllowed creates a new DetailsState if the request is for a valid
// pull request and the user has permissions.
//
// If the state is nil, there was an error or the request is not allowed. In
// this case, getStateIfAllowed writes a response or stores and error in the
// request, so callers can return without doing additional work.
func (h *Details) getStateIfAllowed(w http.ResponseWriter, r *http.Request) *DetailsState {
	ctx := r.Context()

	owner, repo, number, ok := parsePullParams(r)
	if !ok {
		http.Error(w, "Invalid pull request", http.StatusBadRequest)
		return nil
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		if _, notFound := err.(githubapp.InstallationNotFound); notFound {
			h.render404(w, owner, repo, number)
		} else {
			hatpear.Store(r, err)
		}
		return nil
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		hatpear.Store(r, errors.Wrap(err, "failed to create github client"))
		return nil
	}

	user, hasPermission, err := checkUserPermissions(h.Sessions, r, client, owner, repo)
	if err != nil {
		hatpear.Store(r, err)
		return nil
	}
	if !hasPermission {
		h.render404(w, owner, repo, number)
		return nil
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			h.render404(w, owner, repo, number)
		} else {
			hatpear.Store(r, errors.Wrap(err, "failed to get pull request"))
		}
		return nil
	}

	ctx, logger := h.PreparePRContext(ctx, installation.ID, pr)
	evalCtx, err := h.NewEvalContext(ctx, installation.ID, pull.Locator{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		Value:  pr,
	})
	if err != nil {
		hatpear.Store(r, err)
		return nil
	}

	return &DetailsState{
		Ctx:         ctx,
		Logger:      logger,
		EvalContext: evalCtx,
		PullRequest: pr,
		Username:    user,
	}
}

func (h *Details) render(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	return h.Templates.ExecuteTemplate(w, "details.html.tmpl", data)
}

func (h *Details) render404(w http.ResponseWriter, owner, repo string, number int) {
	msg := fmt.Sprintf(
		"Not Found: %s/%s#%d\n\nThe repository or pull request does not exist, you do not have permission, or policy-bot is not installed.",
		owner, repo, number,
	)
	http.Error(w, msg, http.StatusNotFound)
}

func getPolicyURL(pr *github.PullRequest, config FetchedConfig) string {
	base := pr.GetBase().GetRepo().GetHTMLURL()
	if u, _ := url.Parse(base); u != nil {
		srcParts := strings.Split(config.Source, "@")
		if len(srcParts) != 2 {
			return base
		}
		u.Path = path.Join(srcParts[0], "blob", srcParts[1], config.Path)
		return u.String()
	}
	return base
}

func getBasePath(publicURL string) string {
	if u, _ := url.Parse(publicURL); u != nil {
		return strings.TrimSuffix(u.Path, "/")
	}
	return ""
}

func isNotFound(err error) bool {
	rerr, ok := err.(*github.ErrorResponse)
	return ok && rerr.Response.StatusCode == http.StatusNotFound
}

// parsePullParams extracts the pull request paramters from the URL. The final
// boolean parameter is false if any of the parameters are missing or invalid.
func parsePullParams(r *http.Request) (owner, repo string, number int, ok bool) {
	owner = pat.Param(r, "owner")
	repo = pat.Param(r, "repo")

	if n, err := strconv.Atoi(pat.Param(r, "number")); err == nil {
		return owner, repo, n, true
	}
	return owner, repo, 0, false
}

func checkUserPermissions(sessions *scs.Manager, r *http.Request, client *github.Client, owner, repo string) (string, bool, error) {
	username, err := sessions.Load(r).GetString(SessionKeyUsername)
	if err != nil {
		return "", false, errors.Wrap(err, "failed to read sessions")
	}

	level, _, err := client.Repositories.GetPermissionLevel(r.Context(), owner, repo, username)
	if err != nil {
		if isNotFound(err) {
			return username, false, nil
		}
		return "", false, errors.Wrap(err, "failed to get user permission level")
	}
	return username, level.GetPermission() != "none", nil
}
