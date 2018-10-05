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
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/alexedwards/scs"
	"github.com/pkg/errors"
)

var (
	DefaultSessionKey = "oauth2.state"
)

type SessionStateStore struct {
	Sessions *scs.Manager
}

func (s *SessionStateStore) GenerateState(w http.ResponseWriter, r *http.Request) (string, error) {
	sess := s.Sessions.Load(r)

	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, "failed to generate state value")
	}

	state := hex.EncodeToString(b)
	return state, sess.PutString(w, DefaultSessionKey, state)
}

func (s *SessionStateStore) VerifyState(r *http.Request, expected string) (bool, error) {
	sess := s.Sessions.Load(r)

	state, err := sess.GetString(DefaultSessionKey)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare([]byte(expected), []byte(state)) == 1, nil
}
