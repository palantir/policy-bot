// Copyright 2021 Palantir Technologies, Inc.
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

package metrics

import (
	"github.com/rcrowley/go-metrics"
)

var (
	registry metrics.Registry
)

func SetRegistry(r metrics.Registry) {
	registry = r
}

// GitHubCacheApproxSize - registers a gauge with the registry that monitors the approximate GitHub request lrucache memory size
func GitHubCacheApproxSize(sizeFn func() int64) metrics.Gauge {
	return metrics.NewRegisteredFunctionalGauge("github.request_cache.approx_size", registry, sizeFn)
}
