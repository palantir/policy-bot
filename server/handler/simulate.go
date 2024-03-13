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
	"net/http"
	"strings"

	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/simulated"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// Simulate provides a baseline for handlers to perform simulated pull request evaluations and
// either return the result or display it in the ui.
type Simulate struct {
	Base
}

// SimulationResponse is the response returned from Simulate, this is a trimmed down version of common.Result with json
// tags. This struct and the newSimulationResponse constructor can be extended to include extra content from common.Result.
type SimulationResponse struct {
	Name              string `json:"name"`
	Description       string `json:"description:"`
	StatusDescription string `json:"status_description"`
	Status            string `json:"status"`
	Error             string `json:"error"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *Simulate) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	token := getToken(r)
	if token == "" {
		return writeAPIError(w, http.StatusUnauthorized, "missing token")
	}

	client, err := h.NewTokenClient(token)
	if err != nil {
		return errors.Wrap(err, "failed to create token client")
	}

	owner, repo, number, ok := parsePullParams(r)
	if !ok {
		return writeAPIError(w, http.StatusBadRequest, "failed to parse pull request parameters from request")
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			return writeAPIError(w, http.StatusNotFound, "failed to find pull request")
		}

		return errors.Wrap(err, "failed to get pull request")
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		return writeAPIError(w, http.StatusNotFound, "not installed in org")
	}

	ctx, _ = h.PreparePRContext(ctx, installation.ID, pr)
	options, err := simulated.NewOptionsFromRequest(r)
	if err != nil {
		return writeAPIError(w, http.StatusBadRequest, "failed to parse options from request")
	}

	result, err := h.getSimulatedResult(ctx, installation, pull.Locator{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		Value:  pr,
	}, options)

	if err != nil {
		return errors.Wrap(err, "failed to get approval result for pull request")
	}

	response := newSimulationResponse(result)
	baseapp.WriteJSON(w, http.StatusOK, response)
	return nil
}

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return token
	}

	return ""
}

func (h *Simulate) getSimulatedResult(ctx context.Context, installation githubapp.Installation, loc pull.Locator, options simulated.Options) (*common.Result, error) {
	simulatedCtx, config, err := h.newSimulatedContext(ctx, installation.ID, loc, options)
	switch {
	case err != nil:
		return nil, errors.Wrap(err, "failed to generate eval context")
	case config.LoadError != nil:
		return nil, errors.Wrap(config.LoadError, "failed to load policy file")
	case config.ParseError != nil:
		return nil, errors.Wrap(config.ParseError, "failed to parse policy")
	case config.Config == nil:
		// no policy file found on base branch
		return nil, nil
	}

	evaluator, err := policy.ParsePolicy(config.Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get policy evaluator")
	}

	result := evaluator.Evaluate(ctx, simulatedCtx)
	return &result, nil
}

func (h *Simulate) newSimulatedContext(ctx context.Context, installationID int64, loc pull.Locator, options simulated.Options) (*simulated.Context, *FetchedConfig, error) {
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return nil, nil, err
	}

	v4client, err := h.NewInstallationV4Client(installationID)
	if err != nil {
		return nil, nil, err
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, loc.Owner, h.Installations, h.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, h.GlobalCache, client, v4client, loc)
	if err != nil {
		return nil, nil, err
	}

	simulatedPRCtx := simulated.NewContext(ctx, prctx, options)
	baseBranch, _ := simulatedPRCtx.Branches()
	owner := simulatedPRCtx.RepositoryOwner()
	repository := simulatedPRCtx.RepositoryName()
	fetchedConfig := h.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repository, baseBranch)
	return simulatedPRCtx, &fetchedConfig, nil
}

func newSimulationResponse(result *common.Result) *SimulationResponse {
	var response SimulationResponse
	if result != nil {
		if result.Error != nil {
			response.Error = result.Error.Error()
		}

		response.Name = result.Name
		response.Description = result.Description
		response.StatusDescription = result.StatusDescription
		response.Status = result.Status.String()
	}

	return &response
}

func writeAPIError(w http.ResponseWriter, code int, message string) error {
	baseapp.WriteJSON(w, code, ErrorResponse{Error: message})
	return nil
}
