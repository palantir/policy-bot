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
	"net/http"
)

// StateStore generates and verifies the state parameter for OAuth2 flows.
type StateStore interface {
	// GenerateState creates a new state value, storing it in a way that can be
	// retrieved by VerifyState at a later point.
	GenerateState(w http.ResponseWriter, r *http.Request) (string, error)

	// VerifyState checks that the state associated with the request matches
	// the given state. To avoid timing attacks, implementations should use
	// constant-time comparisons if possible.
	VerifyState(r *http.Request, state string) (bool, error)
}

const (
	insecureState = "insecure-for-testing-only"
)

type insecureStateStore struct{}

func (ss insecureStateStore) GenerateState(w http.ResponseWriter, r *http.Request) (string, error) {
	return insecureState, nil
}

func (ss insecureStateStore) VerifyState(r *http.Request, state string) (bool, error) {
	return insecureState == state, nil
}
