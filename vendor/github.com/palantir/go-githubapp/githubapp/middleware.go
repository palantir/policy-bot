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

	"github.com/gregjones/httpcache"
	"github.com/rcrowley/go-metrics"
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
				updateRegistryForHeader(res.Header, httpHeaderRateLimit, metrics.GetOrRegisterGauge(limitMetric, registry))
				updateRegistryForHeader(res.Header, httpHeaderRateRemaining, metrics.GetOrRegisterGauge(remainingMetric, registry))
				// TODO Think about to add X-Ratelimit-Used, X-Ratelimit-Reset and X-Ratelimit-Resource as well
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
