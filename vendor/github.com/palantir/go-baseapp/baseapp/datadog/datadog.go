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

package datadog

import (
	"time"

	"github.com/pkg/errors"
	"github.com/syntaqx/go-metrics-datadog"

	"github.com/palantir/go-baseapp/baseapp"
)

const (
	DefaultAddress  = "127.0.0.1:8125"
	DefaultInterval = 10 * time.Second
)

type Config struct {
	Address  string        `yaml:"address" json:"address"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Tags     []string      `yaml:"tags" json:"tags"`
}

// StartEmitter starts a goroutine that emits metrics from the server's
// registry to the configured DogStatsd endpoint.
func StartEmitter(s *baseapp.Server, c Config) error {
	if c.Address == "" {
		c.Address = DefaultAddress
	}
	if c.Interval == 0 {
		c.Interval = DefaultInterval
	}

	reporter, err := datadog.NewReporter(s.Registry(), c.Address, c.Interval)
	if err != nil {
		return errors.Wrap(err, "datadog: failed to create emitter")
	}

	reporter.Client.Tags = append(reporter.Client.Tags, c.Tags...)
	go reporter.Flush()

	return nil
}
