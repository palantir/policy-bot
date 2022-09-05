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

package oauth2

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

const (
	queryCode  = "code"
	queryError = "error"
	queryState = "state"
)

var (
	ErrInvalidState = errors.New("oauth2: invalid state value")
)

// Login contains information about the result of a successful auth flow.
type Login struct {
	Token  *oauth2.Token
	Client *http.Client
}

// LoginError is an error returned as a parameter by the OAuth provider.
type LoginError string

func (err LoginError) Error() string {
	return string(err)
}

type Param func(*handler)

type ErrorCallback func(w http.ResponseWriter, r *http.Request, err error)
type LoginCallback func(w http.ResponseWriter, r *http.Request, login *Login)

type handler struct {
	config *oauth2.Config

	onError ErrorCallback
	onLogin LoginCallback

	forceTLS bool
	store    StateStore
}

// NewHandler returns an http.Hander that implements the 3-leg OAuth2 flow on a
// single endpoint. It accepts callbacks for both error and success conditions
// so that clients can take action after the auth flow is complete.
func NewHandler(c *oauth2.Config, params ...Param) http.Handler {
	h := &handler{
		config:  c,
		onError: DefaultErrorCallback,
		onLogin: DefaultLoginCallback,
		store:   insecureStateStore{},
	}

	for _, p := range params {
		p(h)
	}

	return h
}

func DefaultErrorCallback(w http.ResponseWriter, r *http.Request, err error) {
	if err == ErrInvalidState {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}
	if _, ok := err.(LoginError); ok {
		http.Error(w, fmt.Sprintf("oauth2 error: %v", err.Error()), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func DefaultLoginCallback(w http.ResponseWriter, r *http.Request, login *Login) {
	w.WriteHeader(http.StatusOK)
}

// ForceTLS determines if generated URLs always use HTTPS. By default, the
// protocol of the request is used.
func ForceTLS(forceTLS bool) Param {
	return func(h *handler) {
		h.forceTLS = forceTLS
	}
}

// WithStore sets the StateStore used to create and verify OAuth2 states. The
// default state store uses a static value, is insecure, and is not suitable
// for production use.
func WithStore(ss StateStore) Param {
	return func(h *handler) {
		h.store = ss
	}
}

// OnError sets the error callback.
func OnError(c ErrorCallback) Param {
	return func(h *handler) {
		h.onError = c
	}
}

// OnLogin sets the login callback.
func OnLogin(c LoginCallback) Param {
	return func(h *handler) {
		h.onLogin = c
	}
}

// WithRedirectURL sets a static redirect URL. By default, the redirect URL is
// generated using the request path, the Host header, and the ForceTLS option.
func WithRedirectURL(uri string) Param {
	return func(h *handler) {
		h.config.RedirectURL = uri
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// copy config for modification
	conf := *h.config
	if conf.RedirectURL == "" {
		conf.RedirectURL = redirectURL(r, h.forceTLS)
	}

	// if the provider returned an error, abort the processes
	if r.FormValue(queryError) != "" {
		h.onError(w, r, LoginError(r.FormValue(queryError)))
		return
	}

	// if this is an initial request, redirect to the provider
	if isInitial(r) {
		state, err := h.store.GenerateState(w, r)
		if err != nil {
			h.onError(w, r, err)
			return
		}

		url := conf.AuthCodeURL(state, oauth2.AccessTypeOnline)
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	// otherwise, verify the state and complete the flow
	isValid, err := h.store.VerifyState(r, r.FormValue(queryState))
	if err != nil {
		h.onError(w, r, err)
		return
	}

	if !isValid {
		h.onError(w, r, ErrInvalidState)
		return
	}

	tok, err := conf.Exchange(r.Context(), r.FormValue(queryCode))
	if err != nil {
		h.onError(w, r, err)
		return
	}

	h.onLogin(w, r, &Login{
		Token:  tok,
		Client: conf.Client(r.Context(), tok),
	})
}

func isInitial(r *http.Request) bool {
	return r.FormValue(queryCode) == ""
}

func redirectURL(r *http.Request, forceTLS bool) string {
	u := *r.URL
	u.Host = r.Host

	if forceTLS || r.TLS != nil {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	q := u.Query()
	q.Del(queryCode)
	q.Del(queryState)
	u.RawQuery = q.Encode()

	return u.String()
}
