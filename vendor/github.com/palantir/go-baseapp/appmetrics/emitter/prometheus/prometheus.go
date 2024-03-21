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

package prometheus

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rcrowley/go-metrics"
)

type Config struct {
	Labels             map[string]string `yaml:"labels" json:"labels"`
	HistogramQuantiles []float64         `yaml:"histogram_quantiles" json:"histogram_quantiles"`
	TimerQuantiles     []float64         `yaml:"timer_quantiles" json:"timer_quantiles"`
}

// NewHandler returns a new http.Handler that returns the metrics in the registry.
func NewHandler(r metrics.Registry, config Config) http.Handler {
	var opts []CollectorOption
	if len(config.Labels) > 0 {
		opts = append(opts, WithLabels(config.Labels))
	}
	if len(config.HistogramQuantiles) > 0 {
		opts = append(opts, WithHistogramQuantiles(config.HistogramQuantiles))
	}
	if len(config.TimerQuantiles) > 0 {
		opts = append(opts, WithTimerQuantiles(config.TimerQuantiles))
	}

	collector := NewCollector(r, opts...)

	promRegistry := prometheus.NewRegistry()
	promRegistry.MustRegister(collector)

	return promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{})
}
