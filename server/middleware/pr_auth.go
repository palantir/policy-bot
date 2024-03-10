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

package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bluekeyes/hatpear"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/server/apierror"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"goji.io/pat"
)

// PullRequestAuth returns middleware that rejects requests if they do not include an `owner`, `repo`
// and `pr` params and a valid GitHub token with read access to the provided pr. Supports both personal
// and server to server tokens.
func PullRequestAuth(cc githubapp.ClientCreator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return hatpear.TryFunc(func(w http.ResponseWriter, r *http.Request) error {
			ctx := r.Context()
			logger := zerolog.Ctx(ctx)

			token := getBearerToken(r)
			if token == "" {
				return apierror.WriteAPIError(w, http.StatusUnauthorized, "missing token")
			}

			owner, repo, prString, err := getPRFromRequest(r)
			if err != nil {
				return apierror.WriteAPIError(w, http.StatusBadRequest, err.Error())
			}

			prNum, err := strconv.Atoi(prString)
			if err != nil {
				logger.Error().Err(err).Msgf("failed to parse pull request number string: %s", prString)
				return apierror.WriteAPIError(w, http.StatusBadRequest, "pr number invalid")
			}

			client, err := cc.NewTokenClient(token)
			if err != nil {
				return errors.Wrap(err, "failed to create token client")
			}

			gitHubPR, _, err := client.PullRequests.Get(r.Context(), owner, repo, prNum)
			if err != nil {
				logger.Error().Err(err).Msg("failed to get pull request")
				return apierror.WriteAPIError(w, http.StatusNotFound, "failed to get pull request")
			}

			if gitHubPR != nil {
				next.ServeHTTP(w, r.WithContext(ctx))
				return nil
			}

			return errors.New("token must be valid have read access to supplied repo")
		})
	}
}

func getBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return token
	}

	return ""
}

func getPRFromRequest(r *http.Request) (string, string, string, error) {
	owner := pat.Param(r, "owner")
	if owner == "" {
		return "", "", "", errors.New("no owner provided")
	}

	repoName := pat.Param(r, "repo")
	if repoName == "" {
		return "", "", "", errors.New("no repo provided")
	}

	prString := pat.Param(r, "number")
	if prString == "" {
		return "", "", "", errors.New("no pr number provided")
	}

	return owner, repoName, prString, nil
}
