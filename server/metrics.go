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

package server

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	gometrics "github.com/rcrowley/go-metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"goji.io"
)

// installationKey is the context key used by go-githubapp to store installation ID.
type installationKey struct{}

// routePatternKey is the context key for storing the matched route pattern.
type routePatternKey struct{}

// Metrics holds all OTEL metrics instruments for the server.
type Metrics struct {
	// HTTP Server metrics
	requestsTotal   metric.Int64Counter
	requestDuration metric.Float64Histogram

	// System metrics (observable gauges registered via callbacks)
	goroutines metric.Int64ObservableGauge
	memoryUsed metric.Int64ObservableGauge

	// GitHub API metrics
	githubRequests      metric.Int64Counter
	githubRateLimit     metric.Int64ObservableGauge
	githubRateRemaining metric.Int64ObservableGauge
	githubRateUsed      metric.Int64ObservableGauge

	// Rate limit state per installation
	rateLimits sync.Map // map[int64]*rateLimitState

	// Event queue metrics
	eventQueueSize metric.Int64ObservableGauge
	eventWorkers   metric.Int64ObservableGauge
	eventAge       metric.Float64Histogram
	eventDropped   metric.Int64Counter

	// Callbacks for observable gauges
	eventQueueSizeFunc func() int64
	eventWorkersFunc   func() int64
	rateLimitCallback  func() []RateLimitInfo
}

// rateLimitState holds rate limit state for a GitHub installation.
type rateLimitState struct {
	limit     int64
	remaining int64
	used      int64
}

// RateLimitInfo holds rate limit information for a GitHub installation.
type RateLimitInfo struct {
	InstallationID int64
	Limit          int64
	Remaining      int64
	Used           int64
}

// NewMetrics creates a new Metrics instance with all OTEL instruments.
func NewMetrics() (*Metrics, error) {
	meter := Meter("policybot")

	m := &Metrics{}

	var err error

	// HTTP Server metrics
	m.requestsTotal, err = meter.Int64Counter(
		"policybot.http.server.requests",
		metric.WithDescription("Total number of HTTP server requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http.server.requests counter: %w", err)
	}

	m.requestDuration, err = meter.Float64Histogram(
		"policybot.http.server.request.duration",
		metric.WithDescription("HTTP server request duration by route"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http.server.request.duration histogram: %w", err)
	}

	// System metrics - registered with callbacks
	m.goroutines, err = meter.Int64ObservableGauge(
		"policybot.process.runtime.goroutines",
		metric.WithDescription("Number of goroutines"),
		metric.WithUnit("{goroutine}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create process.runtime.goroutines gauge: %w", err)
	}

	m.memoryUsed, err = meter.Int64ObservableGauge(
		"policybot.process.runtime.memory.used",
		metric.WithDescription("Memory used by the process"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create process.runtime.memory.used gauge: %w", err)
	}

	// GitHub API metrics
	m.githubRequests, err = meter.Int64Counter(
		"policybot.github.api.requests",
		metric.WithDescription("Total number of GitHub API requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.api.requests counter: %w", err)
	}

	m.githubRateLimit, err = meter.Int64ObservableGauge(
		"policybot.github.api.rate_limit",
		metric.WithDescription("GitHub API rate limit"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.api.rate_limit gauge: %w", err)
	}

	m.githubRateRemaining, err = meter.Int64ObservableGauge(
		"policybot.github.api.rate_remaining",
		metric.WithDescription("GitHub API rate limit remaining"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.api.rate_remaining gauge: %w", err)
	}

	m.githubRateUsed, err = meter.Int64ObservableGauge(
		"policybot.github.api.rate_used",
		metric.WithDescription("GitHub API rate limit used"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.api.rate_used gauge: %w", err)
	}

	// Event queue metrics
	m.eventQueueSize, err = meter.Int64ObservableGauge(
		"policybot.github.event.queue_size",
		metric.WithDescription("Number of events in the queue"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.event.queue_size gauge: %w", err)
	}

	m.eventWorkers, err = meter.Int64ObservableGauge(
		"policybot.github.event.workers",
		metric.WithDescription("Number of active event workers"),
		metric.WithUnit("{worker}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.event.workers gauge: %w", err)
	}

	m.eventAge, err = meter.Float64Histogram(
		"policybot.github.event.age",
		metric.WithDescription("Age of events when processed"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.event.age histogram: %w", err)
	}

	m.eventDropped, err = meter.Int64Counter(
		"policybot.github.event.dropped",
		metric.WithDescription("Number of dropped events"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create github.event.dropped counter: %w", err)
	}

	// Register callbacks for observable gauges
	_, err = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			o.ObserveInt64(m.goroutines, int64(runtime.NumGoroutine()))

			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			o.ObserveInt64(m.memoryUsed, int64(memStats.Alloc))

			return nil
		},
		m.goroutines,
		m.memoryUsed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register system metrics callback: %w", err)
	}

	// Register callback for rate limit metrics from sync.Map
	_, err = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			m.rateLimits.Range(func(key, value any) bool {
				installID := key.(int64)
				state := value.(*rateLimitState)
				attrs := metric.WithAttributes(
					attribute.Int64("installation_id", installID),
				)
				o.ObserveInt64(m.githubRateLimit, state.limit, attrs)
				o.ObserveInt64(m.githubRateRemaining, state.remaining, attrs)
				o.ObserveInt64(m.githubRateUsed, state.used, attrs)
				return true
			})
			return nil
		},
		m.githubRateLimit,
		m.githubRateRemaining,
		m.githubRateUsed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register rate limit metrics callback: %w", err)
	}

	return m, nil
}

// RegisterEventQueueCallback registers a callback function for event queue metrics.
func (m *Metrics) RegisterEventQueueCallback(queueSizeFunc, workersFunc func() int64) error {
	m.eventQueueSizeFunc = queueSizeFunc
	m.eventWorkersFunc = workersFunc

	meter := Meter("policybot")
	_, err := meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			if m.eventQueueSizeFunc != nil {
				o.ObserveInt64(m.eventQueueSize, m.eventQueueSizeFunc())
			}
			if m.eventWorkersFunc != nil {
				o.ObserveInt64(m.eventWorkers, m.eventWorkersFunc())
			}
			return nil
		},
		m.eventQueueSize,
		m.eventWorkers,
	)
	return err
}

// RegisterRateLimitCallback registers a callback for GitHub rate limit metrics.
func (m *Metrics) RegisterRateLimitCallback(callback func() []RateLimitInfo) error {
	m.rateLimitCallback = callback

	meter := Meter("policybot")
	_, err := meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			if m.rateLimitCallback == nil {
				return nil
			}
			for _, info := range m.rateLimitCallback() {
				attrs := metric.WithAttributes(
					attribute.Int64("installation_id", info.InstallationID),
				)
				o.ObserveInt64(m.githubRateLimit, info.Limit, attrs)
				o.ObserveInt64(m.githubRateRemaining, info.Remaining, attrs)
				o.ObserveInt64(m.githubRateUsed, info.Used, attrs)
			}
			return nil
		},
		m.githubRateLimit,
		m.githubRateRemaining,
		m.githubRateUsed,
	)
	return err
}

// RecordRequest records HTTP request metrics.
// It implements baseapp.AccessCallback.
func (m *Metrics) RecordRequest(r *http.Request, status int, size int64, elapsed time.Duration) {
	route := m.extractRoute(r)
	statusClass := statusClass(status)

	attrs := metric.WithAttributes(
		attribute.String("http.route", route),
		attribute.String("http.method", r.Method),
		attribute.Int("http.status_code", status),
		attribute.String("status_class", statusClass),
	)

	m.requestsTotal.Add(r.Context(), 1, attrs)
	m.requestDuration.Record(r.Context(), elapsed.Seconds(), attrs)
}

// extractRoute gets the route pattern from the request context.
func (m *Metrics) extractRoute(r *http.Request) string {
	if pattern, ok := r.Context().Value(routePatternKey{}).(string); ok && pattern != "" {
		return pattern
	}
	return "other"
}

// RecordGitHubRequest records a GitHub API request.
func (m *Metrics) RecordGitHubRequest(ctx context.Context, statusCode int, cached bool) {
	attrs := metric.WithAttributes(
		attribute.Int("http.status_code", statusCode),
		attribute.String("status_class", statusClass(statusCode)),
		attribute.Bool("cached", cached),
	)
	m.githubRequests.Add(ctx, 1, attrs)
}

// RecordEventAge records the age of an event when it's processed.
func (m *Metrics) RecordEventAge(ctx context.Context, age time.Duration) {
	m.eventAge.Record(ctx, age.Seconds())
}

// RecordEventDropped records a dropped event.
func (m *Metrics) RecordEventDropped(ctx context.Context) {
	m.eventDropped.Add(ctx, 1)
}

// statusClass returns the status class (e.g., "2xx", "4xx") for an HTTP status code.
func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "other"
	}
}

// getInstallationID extracts the installation ID from the request context.
// Returns 0 if not found.
func getInstallationID(ctx context.Context) int64 {
	if id, ok := ctx.Value(installationKey{}).(int64); ok {
		return id
	}
	return 0
}

// parseHeaderInt64 parses an HTTP header value as an int64.
// Returns 0 if the header is empty or cannot be parsed.
func parseHeaderInt64(value string) int64 {
	if value == "" {
		return 0
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// updateRateLimits extracts rate limit information from response headers
// and stores it per installation.
func (m *Metrics) updateRateLimits(installationID int64, headers http.Header) {
	limit := parseHeaderInt64(headers.Get("X-Ratelimit-Limit"))
	remaining := parseHeaderInt64(headers.Get("X-Ratelimit-Remaining"))
	used := parseHeaderInt64(headers.Get("X-Ratelimit-Used"))

	if limit > 0 || remaining > 0 || used > 0 {
		m.rateLimits.Store(installationID, &rateLimitState{
			limit:     limit,
			remaining: remaining,
			used:      used,
		})
	}
}

// trackedPattern wraps a goji.Pattern to store the pattern string in the context.
type trackedPattern struct {
	goji.Pattern
	pattern string
}

// Match delegates to the wrapped pattern and adds the pattern string to the context.
func (tp *trackedPattern) Match(r *http.Request) *http.Request {
	matched := tp.Pattern.Match(r)
	if matched != nil {
		ctx := context.WithValue(matched.Context(), routePatternKey{}, tp.pattern)
		return matched.WithContext(ctx)
	}
	return nil
}

// TrackPattern wraps a goji.Pattern to enable route metrics tracking.
// If the pattern implements fmt.Stringer (like pat.Pattern), the pattern
// string is automatically extracted; otherwise provide it via patternStr.
func TrackPattern(p goji.Pattern, patternStr ...string) goji.Pattern {
	var pattern string
	if len(patternStr) > 0 {
		pattern = patternStr[0]
	} else if stringer, ok := p.(fmt.Stringer); ok {
		pattern = stringer.String()
	}
	return &trackedPattern{Pattern: p, pattern: pattern}
}

// GitHubTransport wraps an http.RoundTripper to record GitHub API metrics.
type GitHubTransport struct {
	Base    http.RoundTripper
	Metrics *Metrics
}

// RoundTrip implements http.RoundTripper and records metrics for GitHub API requests.
func (t *GitHubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Get installation ID from context (set by go-githubapp)
	installationID := getInstallationID(req.Context())

	// Check if response was served from cache
	cached := resp.Header.Get("X-From-Cache") == "1"

	t.Metrics.RecordGitHubRequest(req.Context(), resp.StatusCode, cached)

	// Extract and store rate limits (only for non-cached responses)
	if !cached {
		t.Metrics.updateRateLimits(installationID, resp.Header)
	}

	return resp, nil
}

// WrapTransport returns a new GitHubTransport that wraps the given RoundTripper.
func (m *Metrics) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return &GitHubTransport{
		Base:    rt,
		Metrics: m,
	}
}

// ClientMetrics returns a githubapp.ClientMiddleware that records GitHub API metrics.
func (m *Metrics) ClientMetrics() func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return m.WrapTransport(next)
	}
}

// GoMetricsCollector implements prometheus.Collector to expose go-metrics
// registry metrics in Prometheus format.
type GoMetricsCollector struct {
	registry  gometrics.Registry
	namespace string
}

// NewGoMetricsCollector creates a new collector that exposes metrics from
// a go-metrics registry to Prometheus.
func NewGoMetricsCollector(registry gometrics.Registry, namespace string) *GoMetricsCollector {
	return &GoMetricsCollector{
		registry:  registry,
		namespace: namespace,
	}
}

// Describe implements prometheus.Collector.
// We use a dynamic collector pattern, so we don't pre-declare metrics.
func (c *GoMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	// Dynamic collector - we send metrics directly in Collect
}

// Collect implements prometheus.Collector.
func (c *GoMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	c.registry.Each(func(name string, i interface{}) {
		promName := c.promName(name)

		switch m := i.(type) {
		case gometrics.Counter:
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(promName, name, nil, nil),
				prometheus.CounterValue,
				float64(m.Count()),
			)
		case gometrics.Gauge:
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(promName, name, nil, nil),
				prometheus.GaugeValue,
				float64(m.Value()),
			)
		case gometrics.GaugeFloat64:
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(promName, name, nil, nil),
				prometheus.GaugeValue,
				m.Value(),
			)
		case gometrics.Histogram:
			snap := m.Snapshot()
			ch <- prometheus.MustNewConstHistogram(
				prometheus.NewDesc(promName, name, nil, nil),
				uint64(snap.Count()),
				float64(snap.Sum()),
				map[float64]uint64{
					0.5:  uint64(snap.Percentile(0.5)),
					0.9:  uint64(snap.Percentile(0.9)),
					0.99: uint64(snap.Percentile(0.99)),
				},
			)
		case gometrics.Meter:
			snap := m.Snapshot()
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(promName+"_total", name+" total count", nil, nil),
				prometheus.CounterValue,
				float64(snap.Count()),
			)
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(promName+"_rate1", name+" 1-minute rate", nil, nil),
				prometheus.GaugeValue,
				snap.Rate1(),
			)
		case gometrics.Timer:
			snap := m.Snapshot()
			ch <- prometheus.MustNewConstHistogram(
				prometheus.NewDesc(promName, name, nil, nil),
				uint64(snap.Count()),
				float64(snap.Sum()),
				map[float64]uint64{
					0.5:  uint64(snap.Percentile(0.5)),
					0.9:  uint64(snap.Percentile(0.9)),
					0.99: uint64(snap.Percentile(0.99)),
				},
			)
		}
	})
}

// promName converts a go-metrics name to a Prometheus-compatible name.
func (c *GoMetricsCollector) promName(name string) string {
	// Replace dots and dashes with underscores
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")

	if c.namespace != "" {
		return c.namespace + "_" + name
	}
	return name
}
