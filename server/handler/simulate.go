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

	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/simulated"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/server/apierror"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
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

func (h *Simulate) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	logger := *zerolog.Ctx(ctx)

	owner, repo, number, ok := parsePullParams(r)
	if !ok {
		logger.Error().Msg("failed to parse pull request parameters from request")
		return apierror.WriteAPIError(w, http.StatusBadRequest, "failed to parse pull request parameters from request")
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		return errors.Wrap(err, "failed to get installation for org")
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		return errors.Wrap(err, "failed to create github client")
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			logger.Error().Err(err).Msg("could not find pull request")
			return apierror.WriteAPIError(w, http.StatusNotFound, "failed to find pull request")
		}

		return errors.Wrap(err, "failed to get pr")
	}

	ctx, logger = h.PreparePRContext(ctx, installation.ID, pr)
	options, err := simulated.NewOptionsFromRequest(r)
	if err != nil {
		logger.Error().Msg("failed to get options from request")
		return apierror.WriteAPIError(w, http.StatusBadRequest, "failed to parse options from request")
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
		return nil, errors.New("no policy file found in repo")
	}

	evaluator, err := policy.ParsePolicy(config.Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get policy evaluator")
	}

	result := evaluator.Evaluate(ctx, simulatedCtx)
	if result.Error != nil {
		return nil, errors.Wrapf(err, "error evaluating policy in %s: %s", config.Source, config.Path)
	}

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
	var errString string
	if result.Error != nil {
		errString = result.Error.Error()
	}

	return &SimulationResponse{
		Name:              result.Name,
		Description:       result.Description,
		StatusDescription: result.StatusDescription,
		Status:            result.Status.String(),
		Error:             errString,
	}
}
