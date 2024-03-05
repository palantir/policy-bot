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
	"net/url"
	"strings"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/hatpear"
	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/go-githubapp/oauth2"
	"github.com/pkg/errors"
)

const (
	SessionKeyUsername = "username"
	SessionKeyRedirect = "redirect"
)

func Login(c githubapp.Config, basePath string, sessions *scs.Manager) oauth2.LoginCallback {
	return func(w http.ResponseWriter, r *http.Request, login *oauth2.Login) {
		client := github.NewClient(login.Client)

		// TODO(bkeyes): this should be in baseapp or something
		// I should be able to get a valid, parsed URL
		u, err := url.Parse(strings.TrimSuffix(c.V3APIURL, "/") + "/")
		if err != nil {
			hatpear.Store(r, errors.Wrap(err, "failed to parse github url"))
			return
		}
		client.BaseURL = u

		user, _, err := client.Users.Get(r.Context(), "")
		if err != nil {
			hatpear.Store(r, errors.Wrap(err, "failed to get github user"))
			return
		}

		sess := sessions.Load(r)
		if err := sess.PutString(w, SessionKeyUsername, user.GetLogin()); err != nil {
			hatpear.Store(r, errors.Wrap(err, "failed to save session"))
			return
		}

		// go to root or back to the previous page
		target, err := sess.GetString(SessionKeyRedirect)
		if err != nil {
			hatpear.Store(r, errors.Wrap(err, "failed to read session"))
			return
		}
		if target == "" {
			target = basePath + "/"
		}

		http.Redirect(w, r, target, http.StatusFound)
	}
}

func RequireLogin(sessions *scs.Manager, basePath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := sessions.Load(r)

			user, err := sess.GetString(SessionKeyUsername)
			if err != nil {
				hatpear.Store(r, errors.Wrap(err, "failed to read session"))
				return
			}

			if user == "" {
				if err := sess.PutString(w, SessionKeyRedirect, r.URL.String()); err != nil {
					hatpear.Store(r, errors.Wrap(err, "failed to save session"))
					return
				}

				http.Redirect(w, r, basePath+oauth2.DefaultRoute, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
