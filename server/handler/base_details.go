// Copyright 2022 Palantir Technologies, Inc.
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
	"goji.io/pat"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/templatetree"
	"github.com/google/go-github/v45/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type DetailsBase struct {
	Base
	Sessions  *scs.Manager
	Templates templatetree.HTMLTree
}

type DetailsData struct {
	Error            error
	IsTemporaryError bool
	Result           *common.Result
	PullRequest      *github.PullRequest
	User             string
	PolicyURL        string
}

func (h *DetailsBase) getUrlParams(w http.ResponseWriter, r *http.Request) (string, string, int, error) {
	owner := pat.Param(r, "owner")
	repo := pat.Param(r, "repo")
	//number :=
	number, err := strconv.Atoi(pat.Param(r, "number"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid pull request number: %v", err), http.StatusBadRequest)
		return "", "", -1, err
	}

	return owner, repo, number, nil
}

func (h *DetailsBase) generatePrContext(ctx context.Context, owner string, repo string, number int) (pull.Context, error) {
	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		if _, notFound := err.(githubapp.InstallationNotFound); notFound {
			//h.render404(w, owner, repo, number)
			return nil, nil
		}
		return nil, err
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create github client")
	}

	v4client, err := h.ClientCreator.NewInstallationV4Client(installation.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create github client")
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			//h.render404(w, owner, repo, number)
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to get pull request")
	}
	mbrCtx := NewCrossOrgMembershipContext(ctx, client, owner, h.Installations, h.ClientCreator)

	prCtx, err := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, pull.Locator{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		Value:  pr,
	})

	return prCtx, err
}

func (h *DetailsBase) getPolicyConfig(ctx context.Context, prCtx pull.Context, branch string) (FetchedConfig, error) {
	owner := prCtx.RepositoryOwner()

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		if _, notFound := err.(githubapp.InstallationNotFound); notFound {
			//h.render404(w, owner, repo, number)
			err = errors.Wrap(err, "failed to get github installation")
		}
	}
	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		err = errors.Wrap(err, "failed to get github installaion")
		//return nil, errors.Wrap(err, "failed to create github client")
	}

	config := h.ConfigFetcher.ConfigForPRBranch(ctx, prCtx, client, branch)

	return config, err
}

func (h *DetailsBase) generateEvaluationDetails(w http.ResponseWriter, r *http.Request, policyConfig FetchedConfig, prCtx pull.Context) (*DetailsData, *github.Client, common.Evaluator, error) {
	ctx := r.Context()
	//logger := zerolog.Ctx(ctx)

	owner := prCtx.RepositoryOwner()
	repo := prCtx.RepositoryName()
	number := prCtx.Number()

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		if _, notFound := err.(githubapp.InstallationNotFound); notFound {
			h.render404(w, owner, repo, number)
			return nil, nil, nil, errors.Wrap(err, "Unable to find installation")
		}
		return nil, nil, nil, err
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create github client")
	}

	sess := h.Sessions.Load(r)
	user, err := sess.GetString(SessionKeyUsername)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to read sessions")
	}

	level, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, user)
	if err != nil {
		if isNotFound(err) {
			h.render404(w, owner, repo, number)
			return nil, nil, nil, nil
		}
		return nil, nil, nil, errors.Wrap(err, "failed to get user permission level")
	}

	// if the user does not have permission, pretend the repo/PR doesn't exist
	if level.GetPermission() == "none" {
		h.render404(w, owner, repo, number)
		return nil, nil, nil, nil
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			h.render404(w, owner, repo, number)
			return nil, nil, nil, nil
		}
		return nil, nil, nil, errors.Wrap(err, "failed to get pull request")
	}

	var data DetailsData

	data.PullRequest = pr
	data.User = user

	data.PolicyURL = getPolicyURL(pr, policyConfig)

	evaluator, err := h.Base.ValidateFetchedConfig(ctx, prCtx, client, policyConfig, common.TriggerAll)
	if err != nil {
		data.Error = err
		return &data, nil, nil, nil
	}
	if evaluator == nil {
		data.Error = errors.Errorf("Invalid policy at %s: %s", policyConfig.Source, policyConfig.Path)
		return &data, nil, nil, nil
	}

	return &data, client, evaluator, nil
}

func (h *DetailsBase) render404(w http.ResponseWriter, owner, repo string, number int) {
	msg := fmt.Sprintf(
		"Not Found: %s/%s#%DetailsData\n\nThe repository or pull request does not exist, you do not have permission, or policy-bot is not installed.",
		owner, repo, number,
	)
	http.Error(w, msg, http.StatusNotFound)
}

func (h *DetailsBase) render(w http.ResponseWriter, templateName string, data interface{}) error {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	//fmt.Println(data)
	return h.Templates.ExecuteTemplate(w, templateName, data)
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

func isNotFound(err error) bool {
	rerr, ok := err.(*github.ErrorResponse)
	return ok && rerr.Response.StatusCode == http.StatusNotFound
}
