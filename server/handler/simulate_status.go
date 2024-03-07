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
	"net/http"
	"strings"

	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/policy-bot/pull"
	"github.com/rs/zerolog"
)

// SimulateStatus returns a SimulateAPIStatusResponse object as json representing a simulated run of policybot
// on a provided pull request. Accepts query params which may modify the result.
type SimulateStatus struct {
	Simulate
}

type SimulateStatusResponse struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

type APIError struct {
	Status int    `json:"status"`
	Error  string `json:"error"`
}

func (h *SimulateStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := *zerolog.Ctx(ctx)
	var response SimulateStatusResponse

	owner, repo, number, ok := parsePullParams(r)
	if !ok {
		logger.Error().Msg("failed to parse pull request parameters from request")
		writeAPIError(w, http.StatusBadRequest)
		return
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get installation for org")
		writeAPIError(w, http.StatusInternalServerError)
		return
	}

	client, err := h.ClientCreator.NewInstallationClient(installation.ID)
	if err != nil {
		logger.Error().Err(err).Msg("failed to create github client")
		writeAPIError(w, http.StatusInternalServerError)
		return
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get pr")
		if isNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "could not find pull request")
		} else {
			writeAPIError(w, http.StatusInternalServerError)
		}

		return
	}

	ctx, logger = h.PreparePRContext(ctx, installation.ID, pr)
	options := getSimulatedOptions(r)

	result, err := h.getSimulatedResult(ctx, installation, pull.Locator{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		Value:  pr,
	}, options)

	if err != nil {
		logger.Error().Err(err).Msg("failed to get approval result for pull request")
		writeAPIError(w, http.StatusInternalServerError)
		return
	}

	response.Status = result.Status.String()
	response.Description = result.StatusDescription
	baseapp.WriteJSON(w, http.StatusOK, &response)
}

func writeAPIError(w http.ResponseWriter, code int, message ...string) {
	baseapp.WriteJSON(w, code, APIError{Status: code, Error: strings.Join(message, "; ")})
}
