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
)

type Details struct {
	DetailsBase
}

func (h *Details) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	owner, repo, number, err := h.getUrlParams(w, r)
	if err != nil {
		return err
	}

	prCtx, err := h.generatePrContext(ctx, owner, repo, number)
	branch, _ := prCtx.Branches()

	policyConfig, _ := h.getPolicyConfig(ctx, prCtx, branch)
	details, client, evaluator, _ := h.generateEvaluationDetails(w, r, policyConfig, prCtx)

	result, err := h.Base.EvaluateFetchedConfig(ctx, prCtx, client, evaluator, policyConfig)
	details.Result = &result
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	return h.render(w, "details.html.tmpl", details)
}
