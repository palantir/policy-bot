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
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/google/go-github/v90/github"
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
	Name              string                `json:"name"`
	Description       string                `json:"description"`
	StatusDescription string                `json:"status_description"`
	Status            string                `json:"status"`
	Error             string                `json:"error"`
	Children          []*SimulationResponse `json:"children,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *Simulate) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	username, ok, err := h.getRequestUserFromToken(ctx, r)
	if err != nil {
		return err
	}
	if !ok {
		return writeAPIError(w, http.StatusUnauthorized, "missing or invalid token")
	}

	owner, repo, number, ok := parsePullParams(r)
	if !ok {
		return writeAPIError(w, http.StatusBadRequest, "failed to parse pull request parameters from request")
	}

	installation, err := h.Installations.GetByOwner(ctx, owner)
	if err != nil {
		return writeAPI404Error(w)
	}

	client, err := h.NewInstallationClient(installation.ID)
	if err != nil {
		return err
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isNotFound(err) {
			return writeAPI404Error(w)
		}
		return errors.Wrap(err, "failed to get pull request")
	}

	permission, err := getUserSimulatePermission(ctx, client, owner, repo, username)
	if err != nil {
		return err
	}
	switch permission {
	case simulatePermissionSimulate:
		// allowed to simulate, continue with handler
	case simulatePermissionExists:
		return writeAPIError(w, http.StatusForbidden, "simulation is only available to users with admin or maintain permission")
	default:
		return writeAPI404Error(w)
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

	opts := &policy.GlobalOptions{
		IgnoreEditedComments: h.PullOpts.IgnoreEditedComments,
		ApprovalDefaults:     h.PullOpts.ApprovalDefaults,
	}

	evaluator, err := policy.ParsePolicy(config.Config, opts)
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
	policyRef, err := h.policyRef(ctx, client, owner, repository, baseBranch, loc.Value.GetBase().GetRepo().GetDefaultBranch())
	if err != nil {
		return nil, nil, err
	}
	fetchedConfig := h.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repository, policyRef)
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
		response.Children = buildChildren(result.Children)
	}

	return &response
}

func buildChildren(children []*common.Result) []*SimulationResponse {
	if len(children) == 0 {
		return nil
	}
	result := make([]*SimulationResponse, len(children))
	for i, child := range children {
		result[i] = newSimulationResponse(child)
	}
	return result
}

// getRequestUserFromToken uses the GitHub token from the request to look up
// the username of the token owner. It returns an empty username and false if
// the token is missing or invalid. It returns an error only if an error
// occurred while checking the token.
func (h *Simulate) getRequestUserFromToken(ctx context.Context, r *http.Request) (string, bool, error) {
	token := getToken(r)
	if token == "" {
		return "", false, nil
	}

	client, err := h.NewTokenClient(token)
	if err != nil {
		return "", false, errors.Wrap(err, "failed to create token client")
	}

	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		if rerr, ok := stderrors.AsType[*github.ErrorResponse](err); ok {
			switch rerr.Response.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return "", false, nil
			}
		}
		return "", false, errors.Wrap(err, "failed to get authenticated user")
	}

	return user.GetLogin(), true, nil
}

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return token
	}
	return ""
}

type simulatePermission int

const (
	// User has no permission to see this repository
	simulatePermissionNone simulatePermission = iota
	// User can see the repository but cannot run simulations
	simulatePermissionExists
	// User can perform simulations
	simulatePermissionSimulate
)

func getUserSimulatePermission(ctx context.Context, client *github.Client, owner, repo, username string) (simulatePermission, error) {
	level, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, username)
	if err != nil {
		if isNotFound(err) {
			return simulatePermissionNone, nil
		}
		return simulatePermissionNone, errors.Wrap(err, "failed to get user permission level")
	}

	perms := level.GetUser().GetPermissions()
	switch {
	case perms.GetAdmin() || perms.GetMaintain():
		return simulatePermissionSimulate, nil
	case perms.GetPush() || perms.GetPull() || perms.GetTriage():
		return simulatePermissionExists, nil
	}
	return simulatePermissionNone, nil
}

func writeAPI404Error(w http.ResponseWriter) error {
	return writeAPIError(w, http.StatusNotFound, "not found: the repository or pull request does not exist, you do not have permission, or policy-bot is not installed")
}

func writeAPIError(w http.ResponseWriter, code int, message string) error {
	baseapp.WriteJSON(w, code, ErrorResponse{Error: message})
	return nil
}
