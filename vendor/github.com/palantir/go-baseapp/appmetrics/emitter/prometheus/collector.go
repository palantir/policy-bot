// Copyright 2023 Palantir Technologies, Inc.
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

// Package prometheus defines configuration and functions for emitting metrics
// to Prometheus.
//
// It supports a special format for metric names to add metric-specific labels:
//
//	metricName[label1,label2:value2,...]
//
// Global labels for all metrics can be set in the configuration. If a label is
// specified without a value, the label key is used as the value.
//
// The package translates between rcrowley/go-metrics types and Prometheus
// types as neeeded:
//
//   - metrics.Counter metrics are reported as untyped metrics because they may
//     increase or decrease
//   - metrics.Histogram metrics are reported as Prometheus summaries using a
//     configurable (per emitter) set of quantiles. The max and min values are
//     also reported. Use Prometheus functions to compute the mean.
//   - metrics.Meter metrics are reported as Prometheus counters. Use
//     Prometheus functions to compute rates.
//   - metrics.Timers values are reported as Prometheus summaries in fractional
//     seconds using a configurable (per emitter) set of quantiles. The max and
//     min values are also reported. Use Prometheus functions to compute the
//     mean and rates.
package prometheus

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rcrowley/go-metrics"
)

// Collector is a prometheus.Collector that emits the metrics from a
// metrics.Registry.
type Collector struct {
	registry metrics.Registry

	labels             prometheus.Labels
	histogramQuantiles []float64
	timerQuantiles     []float64
}

func NewCollector(r metrics.Registry, opts ...CollectorOption) *Collector {
	c := Collector{
		registry:           r,
		histogramQuantiles: []float64{0.5, 0.95},
		timerQuantiles:     []float64{0.5, 0.95},
	}

	for _, opt := range opts {
		opt(&c)
	}

	return &c
}

type CollectorOption func(*Collector)

// WithLabels sets static labels to attach to all metrics.
func WithLabels(labels map[string]string) CollectorOption {
	return func(c *Collector) {
		c.labels = make(prometheus.Labels, len(labels))
		for k, v := range labels {
			c.labels[sanitizeLabel(k)] = v
		}
	}
}

// WithHistogramQuantiles sets the quantiles reported in summaries of histogram
// metrics. By default, use 0.5 and 0.95, the median and the 95th percentile.
func WithHistogramQuantiles(qs []float64) CollectorOption {
	return func(c *Collector) {
		c.histogramQuantiles = make([]float64, len(qs))
		copy(c.histogramQuantiles, qs)
	}
}

// WithTimerQuantiles sets the quantiles reported in summaries of timer
// metrics. By default, use 0.5 and 0.95, the median and the 95th percentile.
func WithTimerQuantiles(qs []float64) CollectorOption {
	return func(c *Collector) {
		c.timerQuantiles = make([]float64, len(qs))
		copy(c.timerQuantiles, qs)
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	// Send no descriptors to register as an "unchecked" collector: the set of
	// metrics in a go-metrics registry is dynamic, so there's no way to report
	// all of them upfront.
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.registry.Each(func(name string, metric any) {
		switch m := metric.(type) {
		case metrics.Counter:
			desc := c.descFromName(name, "metrics.Counter")
			ch <- prometheus.MustNewConstMetric(desc(""), prometheus.UntypedValue, float64(m.Count()))

		case metrics.Gauge:
			desc := c.descFromName(name, "metrics.Gauge")
			ch <- prometheus.MustNewConstMetric(desc(""), prometheus.GaugeValue, float64(m.Value()))

		case metrics.GaugeFloat64:
			desc := c.descFromName(name, "metrics.GaugeFloat64")
			ch <- prometheus.MustNewConstMetric(desc(""), prometheus.GaugeValue, m.Value())

		case metrics.Histogram:
			desc := c.descFromName(name, "metrics.Histogram")

			ms := m.Snapshot()
			qs := getQuantiles(ms, c.histogramQuantiles)
			ch <- prometheus.MustNewConstSummary(desc(""), uint64(ms.Count()), float64(ms.Sum()), qs)
			ch <- prometheus.MustNewConstMetric(desc("min"), prometheus.UntypedValue, float64(ms.Min()))
			ch <- prometheus.MustNewConstMetric(desc("max"), prometheus.UntypedValue, float64(ms.Max()))

		case metrics.Meter:
			desc := c.descFromName(name, "metrics.Meter")

			ms := m.Snapshot()
			ch <- prometheus.MustNewConstMetric(desc("count"), prometheus.UntypedValue, float64(ms.Count()))

		case metrics.Timer:
			desc := c.descFromName(name, "metrics.Timer")

			ms := m.Snapshot()
			qs := getQuantiles(ms, c.timerQuantiles)
			for q, v := range qs {
				qs[q] = toSeconds(v)
			}

			ch <- prometheus.MustNewConstSummary(desc("seconds"), uint64(ms.Count()), toSeconds(ms.Sum()), qs)
			ch <- prometheus.MustNewConstMetric(desc("min_seconds"), prometheus.UntypedValue, toSeconds(ms.Min()))
			ch <- prometheus.MustNewConstMetric(desc("max_seconds"), prometheus.UntypedValue, toSeconds(ms.Max()))
		}
	})
}

func (c *Collector) descFromName(name string, help string) func(string) *prometheus.Desc {
	name, labels := labelsFromName(name)

	// Add global labels, preferring metric labels if there's a duplicate
	for k, v := range c.labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	return func(suffix string) *prometheus.Desc {
		fqName := name
		if suffix != "" {
			fqName += "_" + suffix
		}
		return prometheus.NewDesc(fqName, help, nil, labels)
	}
}

// labelsFromName extracts the labels from a metric name and returns the
// sanitized base name and the sanitized labels.
func labelsFromName(name string) (string, prometheus.Labels) {
	labels := make(prometheus.Labels)

	start := strings.IndexRune(name, '[')
	if start < 0 || name[len(name)-1] != ']' {
		return sanitizeName(name), labels
	}

	labelPairs := strings.Split(name[start+1:len(name)-1], ",")
	for _, pair := range labelPairs {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if ok {
			labels[sanitizeLabel(key)] = value
		} else {
			labels[sanitizeLabel(key)] = key
		}
	}

	return sanitizeName(name[:start]), labels
}

func sanitizeName(name string) string {
	return sanitize(name, func(c rune) bool {
		return isAlphaNumeric(c) || c == ':'
	})
}

func sanitizeLabel(label string) string {
	return sanitize(label, isAlphaNumeric)
}

func sanitize(v string, valid func(rune) bool) string {
	var clean strings.Builder
	clean.Grow(len(v))

	lastIsValid := false
	for _, c := range v {
		if valid(c) {
			clean.WriteRune(c)
			lastIsValid = true
		} else if lastIsValid {
			clean.WriteByte('_')
			lastIsValid = false
		}
	}

	return clean.String()
}

func isAlphaNumeric(c rune) bool {
	switch {
	case '0' <= c && c <= '9':
	case 'A' <= c && c <= 'Z':
	case 'a' <= c && c <= 'z':
	default:
		return false
	}
	return true
}

func toSeconds[N int64 | float64](n N) float64 {
	return float64(n) / float64(time.Second)
}

type histogram interface {
	Percentiles([]float64) []float64
}

func getQuantiles(metric histogram, ps []float64) map[float64]float64 {
	qs := make(map[float64]float64, len(ps))
	for i, p := range metric.Percentiles(ps) {
		qs[ps[i]] = p
	}
	return qs
}
