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

// Package datadog re-exports appmetrics/emitter/datadog to preserve
// compatibility.
//
// Deprecated: Use the appmetrics/emitter/datadog package instead.
package datadog

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/palantir/go-baseapp/appmetrics/emitter/datadog"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/rcrowley/go-metrics"
)

const (
	DefaultAddress  = datadog.DefaultAddress
	DefaultInterval = datadog.DefaultInterval
)

// SetTimerUnit sets the units used when exporting metrics.Timer metrics. By
// default, times are reported in nanoseconds. You must call this function
// before starting any Emitter instances.
//
// Deprecated: Use the appmetrics/emitter/datadog package instead.
func SetTimerUnit(unit time.Duration) {
	datadog.SetTimerUnit(unit)
}

type Config = datadog.Config

// StartEmitter starts a goroutine that emits metrics from the server's
// registry to the configured DogStatsd endpoint.
//
// Deprecated: Use the appmetrics/emitter/datadog package instead.
func StartEmitter(s *baseapp.Server, c Config) error {
	return datadog.StartEmitter(s, c)
}

type Emitter = datadog.Emitter

// NewEmitter creates a new Emitter that sends metrics from registry using
// client.
//
// Deprecated: Use the appmetrics/emitter/datadog package instead.
func NewEmitter(client *statsd.Client, registry metrics.Registry) *Emitter {
	return datadog.NewEmitter(client, registry)
}
