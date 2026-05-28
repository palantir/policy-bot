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

package githubclient

import (
	"context"
	"io"
	"net/http"

	"github.com/palantir/go-githubapp/githubapp"
	"github.com/rs/zerolog"
)

type retriedKey struct{}

// NewRetryOn401Middleware returns a ClientMiddleware that, on a 401 response
// from GitHub, invalidates the cached installation client, obtains a fresh one
// (whose ghinstallation.Transport will fetch a new token), and replays the
// request once. A context guard prevents infinite recursion.
func NewRetryOn401Middleware(invalidator *InvalidatingClientCreator) githubapp.ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return &retryOn401Transport{
			next:        next,
			invalidator: invalidator,
		}
	}
}

type retryOn401Transport struct {
	next        http.RoundTripper
	invalidator *InvalidatingClientCreator
}

func (t *retryOn401Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	// Already retried once — return the 401 to avoid infinite loops.
	if req.Context().Value(retriedKey{}) != nil {
		return resp, nil
	}

	installationID, ok := InstallationIDFromContext(req.Context())
	if !ok {
		return resp, nil
	}

	// Rebuild the request body for the replay. If the caller didn't provide
	// GetBody (which net/http sets for byte/string/buffer-backed bodies), we
	// can't replay; surface the 401 unchanged.
	getBody := req.GetBody
	if req.Body != nil && req.Body != http.NoBody && getBody == nil {
		return resp, nil
	}

	zerolog.Ctx(req.Context()).Warn().
		Int64("installation_id", installationID).
		Str("url", req.URL.String()).
		Msg("GitHub returned 401, invalidating installation client and retrying once")

	// Drain and close the 401 body before discarding the response.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	t.invalidator.Invalidate(installationID)

	freshClient, err := t.invalidator.NewInstallationClient(installationID)
	if err != nil {
		return nil, err
	}

	req2 := req.Clone(context.WithValue(req.Context(), retriedKey{}, true))
	if getBody != nil {
		body, err := getBody()
		if err != nil {
			return nil, err
		}
		req2.Body = body
	}

	return freshClient.Client().Do(req2)
}
