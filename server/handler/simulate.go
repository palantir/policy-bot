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

	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/simulated"
	"github.com/pkg/errors"
)

const (
	ignoreParam  = "ignore"
	commentParam = "comment"
	reviewParam  = "review"
	branchParam  = "branch"
)

// Simulate provides a baseline for handlers to perform simulated pull request evaluations and
// either return the result or display it in the ui.
type Simulate struct {
	Base
}

func (h *Simulate) getSimulatedResult(ctx context.Context, installation githubapp.Installation, loc pull.Locator, options simulated.Options) (*common.Result, error) {
	evalCtx, err := h.newSimulatedEvalContext(ctx, installation.ID, loc, options)
	switch {
	case err != nil:
		return nil, errors.Wrap(err, "failed to generate eval context")
	case evalCtx.Config.LoadError != nil:
		return nil, errors.Wrap(evalCtx.Config.LoadError, "failed to load policy file")
	case evalCtx.Config.ParseError != nil:
		return nil, errors.Wrap(evalCtx.Config.ParseError, "failed to parse policy")
	case evalCtx.Config.Config == nil:
		return nil, errors.New("no policy file found in repo")
	}

	evaluator, err := policy.ParsePolicy(evalCtx.Config.Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get policy evaluator")
	}

	result := evaluator.Evaluate(ctx, evalCtx.PullContext)
	if result.Error != nil {
		return nil, errors.Wrapf(err, "error evaluating policy in %s: %s", evalCtx.Config.Source, evalCtx.Config.Path)
	}

	return &result, nil
}

func (h *Simulate) newSimulatedEvalContext(ctx context.Context, installationID int64, loc pull.Locator, options simulated.Options) (*EvalContext, error) {
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	v4client, err := h.NewInstallationV4Client(installationID)
	if err != nil {
		return nil, err
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, loc.Owner, h.Installations, h.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, h.GlobalCache, client, v4client, loc)
	if err != nil {
		return nil, err
	}

	simulatedPRCtx := simulated.NewContext(prctx, options)

	baseBranch, _ := simulatedPRCtx.Branches()
	owner := simulatedPRCtx.RepositoryOwner()
	repository := simulatedPRCtx.RepositoryName()

	fetchedConfig := h.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repository, baseBranch)

	return &EvalContext{
		Client:   client,
		V4Client: v4client,

		Options:   h.PullOpts,
		PublicURL: h.BaseConfig.PublicURL,

		PullContext: simulatedPRCtx,
		Config:      fetchedConfig,
	}, nil
}

func getSimulatedOptions(r *http.Request) simulated.Options {
	var options simulated.Options
	if r.URL.Query().Has(ignoreParam) {
		options.Ignore = r.URL.Query().Get(ignoreParam)
	}

	if r.URL.Query().Has(commentParam) {
		options.AddApprovalComment = r.URL.Query().Get(commentParam)
	}

	if r.URL.Query().Has(reviewParam) {
		options.AddApprovalReview = r.URL.Query().Get(reviewParam)
	}

	if r.URL.Query().Has(branchParam) {
		options.BaseBranch = r.URL.Query().Get(branchParam)
	}

	return options
}
