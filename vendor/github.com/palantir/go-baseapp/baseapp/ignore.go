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

package baseapp

import (
	"context"
	"net/http"
)

type ignoreCtxKey struct{}

var (
	zeroRule IgnoreRule
)

// NewIgnoreHandler returns middleware that tracks whether specific requests
// should be ignored in logs and metrics.
func NewIgnoreHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rule IgnoreRule
			r = r.WithContext(context.WithValue(r.Context(), ignoreCtxKey{}, &rule))
			next.ServeHTTP(w, r)
		})
	}
}

// IgnoreRule specifies which types of reporting to ignore for a particular
// request.
type IgnoreRule struct {
	// If true, do not log this request
	Logs bool

	// If true, do not report metrics for this request
	Metrics bool
}

// Ignore sets reporting to ignore for a request. Use this to disable logging
// or metrics for particular request types, like health checks. Call Ignore
// with an empty rule to re-enable reporting for a request that was previously
// ignored.
//
// Ignore only works if the middleware returned by NewIgnoreHandler is used
// before the handler and before any reporting middleware in the middleware
// stack. If this middleware does not exist, Ignore has no effect.
func Ignore(r *http.Request, rule IgnoreRule) {
	ctxRule, ok := r.Context().Value(ignoreCtxKey{}).(*IgnoreRule)
	if ok {
		*ctxRule = rule
	}
}

// IgnoreAll is equivalent to calling Ignore with an IgnoreRule that ignores
// all possible reporting.
func IgnoreAll(r *http.Request) {
	Ignore(r, IgnoreRule{
		Logs:    true,
		Metrics: true,
	})
}

// IsIgnored returns true if the request ignores all of the reporting types set
// in rule. The request may also ignore other reporting types not set in rule.
func IsIgnored(r *http.Request, rule IgnoreRule) bool {
	if rule == zeroRule {
		return false
	}

	ctxRule, ok := r.Context().Value(ignoreCtxKey{}).(*IgnoreRule)
	if ok {
		if rule.Logs && !ctxRule.Logs {
			return false
		}
		if rule.Metrics && !ctxRule.Metrics {
			return false
		}
		return true
	}
	return false
}
