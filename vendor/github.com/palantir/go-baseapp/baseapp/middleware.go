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

package baseapp

import (
	"net/http"
	"time"

	"github.com/bluekeyes/hatpear"
	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

// DefaultMiddleware returns the default middleware stack. The stack:
//
//   - Adds a logger to request contexts
//   - Adds a metrics registry to request contexts
//   - Adds a request ID to all requests and responses
//   - Logs and records metrics for requests, respecting ignore rules
//   - Handles errors returned by route handlers
//   - Recovers from panics in route handlers
//
// All components are exported so users can select individual middleware to
// build their own stack if desired.
func DefaultMiddleware(logger zerolog.Logger, registry metrics.Registry) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		hlog.NewHandler(logger),
		NewMetricsHandler(registry),
		hlog.RequestIDHandler("rid", "X-Request-ID"),
		NewIgnoreHandler(),
		AccessHandler(RecordRequest),
		hatpear.Catch(HandleRouteError),
		hatpear.Recover(),
	}
}

// NewMetricsHandler returns middleware that add the given metrics registry to
// the request context.
func NewMetricsHandler(registry metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithMetricsCtx(r.Context(), registry))
			next.ServeHTTP(w, r)
		})
	}
}

// LogRequest is an AccessCallback that logs request information.
func LogRequest(r *http.Request, status int, size int64, elapsed time.Duration) {
	if IsIgnored(r, IgnoreRule{Logs: true}) {
		return
	}

	hlog.FromRequest(r).Info().
		Str("method", r.Method).
		Str("path", r.URL.String()).
		Str("client_ip", r.RemoteAddr).
		Int("status", status).
		Int64("size", size).
		Dur("elapsed", elapsed).
		Str("user_agent", r.UserAgent()).
		Msg("http_request")
}

// RecordRequest is an AccessCallback that logs request information and
// records request metrics.
func RecordRequest(r *http.Request, status int, size int64, elapsed time.Duration) {
	LogRequest(r, status, size, elapsed)
	CountRequest(r, status, size, elapsed)
}

type AccessCallback func(r *http.Request, status int, size int64, duration time.Duration)

// AccessHandler returns a handler that call f after each request.
func AccessHandler(f AccessCallback) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := WrapWriter(w)
			next.ServeHTTP(wrapped, r)
			f(r, wrapped.Status(), wrapped.BytesWritten(), time.Since(start))
		})
	}
}
