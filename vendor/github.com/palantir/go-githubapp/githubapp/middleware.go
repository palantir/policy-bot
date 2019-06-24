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

package githubapp

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gregjones/httpcache"
	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
)

const (
	MetricsKeyRequests    = "github.requests"
	MetricsKeyRequests2xx = "github.requests.2xx"
	MetricsKeyRequests3xx = "github.requests.3xx"
	MetricsKeyRequests4xx = "github.requests.4xx"
	MetricsKeyRequests5xx = "github.requests.5xx"

	MetricsKeyRequestsCached = "github.requests.cached"

	MetricsKeyRateLimit          = "github.rate.limit"
	MetricsKeyRateLimitRemaining = "github.rate.remaining"
)

// ClientMetrics creates client middleware that records metrics about all
// requests. It also defines the metrics in the provided registry.
func ClientMetrics(registry metrics.Registry) ClientMiddleware {
	for _, key := range []string{
		MetricsKeyRequests,
		MetricsKeyRequests2xx,
		MetricsKeyRequests3xx,
		MetricsKeyRequests4xx,
		MetricsKeyRequests5xx,
		MetricsKeyRequestsCached,
	} {
		// Use GetOrRegister for thread-safety when creating multiple
		// RoundTrippers that share the same registry
		metrics.GetOrRegisterCounter(key, registry)
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			installationID, ok := r.Context().Value(installationKey).(int64)
			if !ok {
				installationID = 0
			}

			res, err := next.RoundTrip(r)

			if res != nil {
				registry.Get(MetricsKeyRequests).(metrics.Counter).Inc(1)
				if key := bucketStatus(res.StatusCode); key != "" {
					registry.Get(key).(metrics.Counter).Inc(1)
				}

				if res.Header.Get(httpcache.XFromCache) != "" {
					registry.Get(MetricsKeyRequestsCached).(metrics.Counter).Inc(1)
				}

				limitMetric := fmt.Sprintf("%s[installation:%d]", MetricsKeyRateLimit, installationID)
				remainingMetric := fmt.Sprintf("%s[installation:%d]", MetricsKeyRateLimitRemaining, installationID)

				// Headers from https://developer.github.com/v3/#rate-limiting
				updateRegistryForHeader(res.Header, "X-RateLimit-Limit", metrics.GetOrRegisterGauge(limitMetric, registry))
				updateRegistryForHeader(res.Header, "X-RateLimit-Remaining", metrics.GetOrRegisterGauge(remainingMetric, registry))
			}

			return res, err
		})
	}
}

func updateRegistryForHeader(headers http.Header, header string, metric metrics.Gauge) {
	headerString := headers.Get(header)
	if headerString != "" {
		headerVal, err := strconv.ParseInt(headerString, 10, 64)
		if err == nil {
			metric.Update(headerVal)
		}
	}
}

func bucketStatus(status int) string {
	switch {
	case status >= 200 && status < 300:
		return MetricsKeyRequests2xx
	case status >= 300 && status < 400:
		return MetricsKeyRequests3xx
	case status >= 400 && status < 500:
		return MetricsKeyRequests4xx
	case status >= 500 && status < 600:
		return MetricsKeyRequests5xx
	}
	return ""
}

// ClientLogging creates client middleware that logs request and response
// information at the given level. If the request fails without creating a
// response, it is logged with a status code of -1. The middleware uses a
// logger from the request context.
func ClientLogging(lvl zerolog.Level) ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			start := time.Now()
			res, err := next.RoundTrip(r)
			elapsed := time.Now().Sub(start)

			log := zerolog.Ctx(r.Context())
			if res != nil {
				log.WithLevel(lvl).
					Str("method", r.Method).
					Str("path", r.URL.String()).
					Int("status", res.StatusCode).
					Int64("size", res.ContentLength).
					Dur("elapsed", elapsed).
					Msg("github_request")
			} else {
				log.WithLevel(lvl).
					Str("method", r.Method).
					Str("path", r.URL.String()).
					Int("status", -1).
					Int64("size", -1).
					Dur("elapsed", elapsed).
					Msg("github_request")
			}

			return res, err
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
