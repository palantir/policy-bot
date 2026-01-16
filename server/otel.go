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
	"net/http"

	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/version"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const (
	defaultServiceName = "policy-bot"
)

// OTELProvider holds the tracer and meter providers and provides shutdown functionality.
type OTELProvider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	promRegistry   *prometheus.Registry
	promHandler    http.Handler
}

// PrometheusHandler returns the HTTP handler for serving Prometheus metrics.
func (p *OTELProvider) PrometheusHandler() http.Handler {
	if p == nil {
		return nil
	}
	return p.promHandler
}

// PromRegistry returns the Prometheus registry for registering additional collectors.
func (p *OTELProvider) PromRegistry() *prometheus.Registry {
	if p == nil {
		return nil
	}
	return p.promRegistry
}

// InitOTEL initializes OpenTelemetry with autoexport for traces and Prometheus for metrics.
// It returns an OTELProvider that includes a Prometheus HTTP handler for mounting on the main server.
// If OTEL is disabled in config, it returns nil with no error.
func InitOTEL(ctx context.Context, cfg *OTELConfig) (*OTELProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version.GetVersion()),
		),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OTEL resource")
	}

	// Use autoexport to create span exporter based on environment variables.
	// This respects OTEL_TRACES_EXPORTER and OTEL_EXPORTER_OTLP_ENDPOINT.
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OTEL span exporter")
	}

	// Create tracer provider with batch span processor
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)

	// Set as global tracer provider
	otel.SetTracerProvider(tracerProvider)

	// Create Prometheus registry and exporter for serving metrics on main server
	promRegistry := prometheus.NewRegistry()
	exporter, err := promexporter.New(
		promexporter.WithRegisterer(promRegistry),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create prometheus exporter")
	}

	// Create meter provider with prometheus exporter
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)

	// Set as global meter provider
	otel.SetMeterProvider(meterProvider)

	// Set up propagator for distributed tracing context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create prometheus HTTP handler
	promHandler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{})

	return &OTELProvider{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		promRegistry:   promRegistry,
		promHandler:    promHandler,
	}, nil
}

// Shutdown gracefully shuts down the OTEL providers.
// It flushes any remaining spans and metrics before closing.
func (p *OTELProvider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to shutdown meter provider"))
		}
	}
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to shutdown tracer provider"))
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Meter returns a meter from the global meter provider for recording metrics.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// ClientTracing creates a githubapp.ClientMiddleware that instruments
// outgoing HTTP requests with OpenTelemetry tracing.
func ClientTracing(enabled bool) githubapp.ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		if !enabled {
			return next
		}
		// otelhttp.NewTransport wraps the transport and creates spans for each request.
		// It automatically captures HTTP method, URL, status code, and request duration.
		return otelhttp.NewTransport(
			next,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return "GitHub API: " + r.Method + " " + r.URL.Path
			}),
		)
	}
}

// WrapHandler wraps an http.Handler with OpenTelemetry instrumentation.
// This should be used to wrap the root mux for server-side tracing.
func WrapHandler(h http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(h, operation)
}
