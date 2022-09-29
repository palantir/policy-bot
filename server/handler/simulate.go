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
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type Simulate struct {
	DetailsBase
}

type SimulatePullContext struct {
	pull.Context

	additionalApprovals []string
}

func (f *SimulatePullContext) Reviews() ([]*pull.Review, error) {
	reviews, err := f.Context.Reviews()

	for _, username := range f.additionalApprovals {
		reviews = append(reviews, &pull.Review{CreatedAt: time.Now(), UpdatedAt: time.Now(), Author: username, State: pull.ReviewApproved})
	}

	return reviews, err
}

func newSimulatePullContext(ctx pull.Context) *SimulatePullContext {
	return &SimulatePullContext{Context: ctx}
}

func (f *SimulatePullContext) addApproval(username string) {
	f.additionalApprovals = append(f.additionalApprovals, username)
}

func (h *Simulate) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	owner, repo, number, err := h.getPrDetails(w, r)
	if err != nil {
		return err
	}

	queryParams := r.URL.Query()
	prCtx, err := h.generatePrContext(ctx, owner, repo, number)
	var branch, username string

	if queryParams.Has("branch") {
		branch = queryParams.Get("branch")
	} else {
		branch, _ = prCtx.Branches()
	}

	if queryParams.Has("username") && queryParams.Get("username") != "" {
		username = queryParams.Get("username")
		fakePrContext := newSimulatePullContext(prCtx)
		fakePrContext.addApproval(username)
		prCtx = fakePrContext
	}

	policyConfig, _ := h.getPolicyConfig(ctx, prCtx, branch)
	if err != nil {
		h.render404(w, owner, repo, number)
		return errors.Wrap(err, "failed to get policy config")
	}
	details, client, evaluator, err := h.generateEvaluationDetails(w, r, policyConfig, prCtx)
	if err != nil {
		h.render404(w, owner, repo, number)
	}

	result := h.Base.EvaluateConfig(ctx, prCtx, client, evaluator, policyConfig)
	details.Result = &result

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	return h.render(w, "simulate.html.tmpl", details)
}
