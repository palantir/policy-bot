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

	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
)

const (
	// nanoTimeFormat is the same as time.RFC3339Nano, but uses a consistent
	// number of digits instead of removing trailing zeros
	nanoTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"
)

func DefaultParams(logger zerolog.Logger, metricsPrefix string) []Param {
	var registry metrics.Registry
	if metricsPrefix == "" {
		registry = metrics.NewRegistry()
	} else {
		registry = metrics.NewPrefixedRegistry(metricsPrefix)
	}

	return []Param{
		WithLogger(logger),
		WithRegistry(registry),
		WithMiddleware(DefaultMiddleware(logger, registry)...),
		WithUTCNanoTime(),
		WithErrorLogging(RichErrorMarshalFunc),
		WithMetrics(10 * time.Second),
	}
}

func WithLogger(logger zerolog.Logger) Param {
	return func(b *Server) error {
		b.logger = logger
		return nil
	}
}

func WithMiddleware(middleware ...func(http.Handler) http.Handler) Param {
	return func(b *Server) error {
		b.middleware = middleware
		return nil
	}
}

func WithUTCNanoTime() Param {
	return func(b *Server) error {
		zerolog.TimeFieldFormat = nanoTimeFormat
		zerolog.TimestampFunc = func() time.Time {
			return time.Now().UTC()
		}
		return nil
	}
}

func WithErrorLogging(marshalFunc func(err error) interface{}) Param {
	return func(b *Server) error {
		zerolog.ErrorMarshalFunc = marshalFunc
		return nil
	}
}

func WithRegistry(registry metrics.Registry) Param {
	return func(b *Server) error {
		b.registry = registry
		return nil
	}
}

func WithMetrics(interval time.Duration) Param {
	return func(s *Server) error {
		s.initMetrics = func() {
			r := s.Registry()
			RegisterDefaultMetrics(r)

			go collectGoMetrics(r, interval)
		}
		return nil
	}
}
