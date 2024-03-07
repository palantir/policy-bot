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
	"strings"

	"github.com/bluekeyes/hatpear"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"goji.io/pat"
)

type TokenValidator interface {
	// ValidateRepoAccess checks that the passed in token is valid and that it can read the repo.
	ValidateRepoAccess(r *http.Request, token string) (valid bool, err error)
}

// RepoAuth returns middleware that rejects requests if they do not include an `owner` and `repo`
// param and a valid GitHub token with access to the provided repo. Supports both personal and
// server to server tokens.
func RepoAuth(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return hatpear.TryFunc(func(w http.ResponseWriter, r *http.Request) error {
			ctx := r.Context()

			token := getBearerToken(r)
			if token == "" {
				return errors.New("missing token")
			}

			valid, err := validator.ValidateRepoAccess(r, token)
			if err != nil {
				return errors.Wrap(err, "failed to validate token")
			}

			if valid {
				next.ServeHTTP(w, r.WithContext(ctx))
				return nil
			}

			return errors.New("token must be valid have read access to supplied repo")
		})
	}
}

type GitHubTokenValidator struct {
	ClientCreator githubapp.ClientCreator
}

func (v *GitHubTokenValidator) ValidateRepoAccess(r *http.Request, token string) (bool, error) {
	owner := pat.Param(r, "owner")
	if owner == "" {
		return false, errors.New("no owner provided")
	}

	repoName := pat.Param(r, "repo")
	if repoName == "" {
		return false, errors.New("no repo provided")
	}

	client, err := v.ClientCreator.NewTokenClient(token)
	if err != nil {
		return false, errors.Wrap(err, "failed to create token client")
	}

	// validating that the client can read the repo works for both oauth and server to server tokens
	repo, _, err := client.Repositories.Get(r.Context(), owner, repoName)
	if err != nil {
		return false, errors.Wrap(err, "failed to get repo")
	}

	return repo.GetOwner().GetLogin() == owner && repo.GetName() == repoName, nil
}

func getBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return token
	}

	return ""
}
