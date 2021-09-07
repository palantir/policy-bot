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
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/version"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"
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

		validLocal, localStrErr := isValidLocalPolicy(requestPolicy)
		validRemote, remoteStrErr := isValidRemotePolicy(requestPolicy)

		if validLocal || validRemote {
			check.Message = "Policy file is valid"
			baseapp.WriteJSON(w, http.StatusOK, &check)
			return
		}

		check.Message = fmt.Sprintf("Policy is invalid and neither local nor remote. '%s' OR '%s'.", localStrErr, remoteStrErr)
		baseapp.WriteJSON(w, http.StatusBadRequest, &check)
		return
	})
}

func isValidLocalPolicy(requestPolicy []byte) (bool, string) {
	var policyConfig policy.Config

	if err := yaml.UnmarshalStrict(requestPolicy, &policyConfig); err != nil {
		return false, err.Error()
	}

	if _, err := policy.ParsePolicy(&policyConfig); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func isValidRemotePolicy(requestPolicy []byte) (bool, string) {
	remoteRef, err := appconfig.YAMLRemoteRefParser("", requestPolicy)
	if err != nil {
		return false, err.Error()
	}

	return remoteRef != nil, ""
}
