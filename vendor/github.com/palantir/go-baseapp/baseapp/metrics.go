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
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/rcrowley/go-metrics"
)

const (
	MetricsKeyRequests    = "server.requests"
	MetricsKeyRequests2xx = "server.requests.2xx"
	MetricsKeyRequests3xx = "server.requests.3xx"
	MetricsKeyRequests4xx = "server.requests.4xx"
	MetricsKeyRequests5xx = "server.requests.5xx"

	MetricsKeyNumGoroutines = "server.goroutines"
	MetricsKeyMemoryUsed    = "server.mem.used"
)

type metricsCtxKey struct{}

// MetricsCtx retries a metrics registry from the context. It returns the
// default registry from the go-metrics package if none exists in the context.
func MetricsCtx(ctx context.Context) metrics.Registry {
	if r, ok := ctx.Value(metricsCtxKey{}).(metrics.Registry); ok {
		return r
	}
	return metrics.DefaultRegistry
}

// WithMetricsCtx stores a metrics registry in a context.
func WithMetricsCtx(ctx context.Context, registry metrics.Registry) context.Context {
	return context.WithValue(ctx, metricsCtxKey{}, registry)
}

// RegisterDefaultMetrics adds the default metrics provided by this package to
// the registry. This should be called before any functions that emit metrics
// to ensure no events are lost.
func RegisterDefaultMetrics(registry metrics.Registry) {
	for _, key := range []string{
		MetricsKeyRequests,
		MetricsKeyRequests2xx,
		MetricsKeyRequests3xx,
		MetricsKeyRequests4xx,
		MetricsKeyRequests5xx,
	} {
		metrics.GetOrRegisterCounter(key, registry)
	}

	metrics.GetOrRegisterGauge(MetricsKeyNumGoroutines, registry)
	metrics.GetOrRegisterGauge(MetricsKeyMemoryUsed, registry)
}

func CountRequest(r *http.Request, status, _ int, _ time.Duration) {
	registry := MetricsCtx(r.Context())

	if c := registry.Get(MetricsKeyRequests); c != nil {
		c.(metrics.Counter).Inc(1)
	}

	if key := bucketStatus(status); key != "" {
		if c := registry.Get(key); c != nil {
			c.(metrics.Counter).Inc(1)
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

func collectGoMetrics(registry metrics.Registry, interval time.Duration) {
	var memStats runtime.MemStats

	ticker := time.Tick(interval)
	for range ticker {
		if g := registry.Get(MetricsKeyNumGoroutines); g != nil {
			num := runtime.NumGoroutine()
			g.(metrics.Gauge).Update(int64(num))
		}

		if g := registry.Get(MetricsKeyMemoryUsed); g != nil {
			runtime.ReadMemStats(&memStats)
			g.(metrics.Gauge).Update(int64(memStats.Alloc))
		}
	}
}
