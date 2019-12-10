// Copyright 2019 Palantir Technologies, Inc.
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
	"io/ioutil"
	"net/http"

	"github.com/palantir/go-baseapp/baseapp"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/version"
)

type ValidateCheck struct {
	Message string `json:"message"`
	Version string `json:"version"`
}

func Validate() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := zerolog.Ctx(ctx)

		logger.Info().Msg("Attempting to validate policy file")
		check := ValidateCheck{Version: version.GetVersion()}

		requestPolicy, err := ioutil.ReadAll(r.Body)
		if err != nil {
			check.Message = "Unable to read policy file buffer"
			baseapp.WriteJSON(w, http.StatusInternalServerError, &check)
			return
		}

		var policyConfig policy.Config
		err = yaml.UnmarshalStrict(requestPolicy, &policyConfig)
		if err != nil {
			check.Message = err.Error()
			baseapp.WriteJSON(w, http.StatusBadRequest, &check)
			return
		}

		_, err = policy.ParsePolicy(&policyConfig)
		if err != nil {
			check.Message = err.Error()
			baseapp.WriteJSON(w, http.StatusUnprocessableEntity, &check)
			return
		}

		check.Message = "Policy file is valid"
		baseapp.WriteJSON(w, http.StatusOK, &check)
	})
}
