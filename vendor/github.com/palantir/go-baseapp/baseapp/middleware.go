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

func DefaultMiddleware(logger zerolog.Logger, registry metrics.Registry) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		hlog.NewHandler(logger),
		NewMetricsHandler(registry),
		hlog.RequestIDHandler("rid", "X-Request-ID"),
		hlog.AccessHandler(RecordRequest),
		hatpear.Catch(HandleRouteError),
		hatpear.Recover(),
	}
}

func NewMetricsHandler(registry metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(WithMetricsCtx(r.Context(), registry))
			next.ServeHTTP(w, r)
		})
	}
}

func LogRequest(r *http.Request, status, size int, elapsed time.Duration) {
	hlog.FromRequest(r).Info().
		Str("method", r.Method).
		Str("path", r.URL.String()).
		Int("status", status).
		Int("size", size).
		Dur("elapsed", elapsed).
		Msg("http_request")
}

func RecordRequest(r *http.Request, status, size int, elapsed time.Duration) {
	LogRequest(r, status, size, elapsed)
	CountRequest(r, status, size, elapsed)
}
