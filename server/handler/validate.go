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
	"bytes"
	"io"
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
		file, _, err := r.FormFile("policy")
		if err != nil {
			baseapp.WriteJSON(w, http.StatusBadRequest, &ValidateCheck{Message: "Unable to read policy file from request", Version: version.GetVersion()})
			return
		}
		defer func() {
			ferr := file.Close()
			if ferr != nil {
				logger.Error().Err(ferr).Msg("Unable to close file")
			}
		}()

		var policyBuf bytes.Buffer
		_, err = io.Copy(&policyBuf, file)
		if err != nil {
			baseapp.WriteJSON(w, http.StatusBadRequest, &ValidateCheck{Message: "Unable to read policy file buffer", Version: version.GetVersion()})
			return
		}

		var policyConfig policy.Config
		err = yaml.UnmarshalStrict(policyBuf.Bytes(), &policyConfig)
		if err != nil {
			baseapp.WriteJSON(w, http.StatusBadRequest, &ValidateCheck{Message: err.Error(), Version: version.GetVersion()})
			return
		}

		_, err = policy.ParsePolicy(&policyConfig)
		if err != nil {
			baseapp.WriteJSON(w, http.StatusBadRequest, &ValidateCheck{Message: err.Error(), Version: version.GetVersion()})
			return
		}

		baseapp.WriteJSON(w, http.StatusOK, &ValidateCheck{Message: "Policy file is valid", Version: version.GetVersion()})
	})
}
