// Copyright 2022 Palantir Technologies, Inc.
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

package githubapp

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gregjones/httpcache"
	"github.com/rs/zerolog"
)

const (
	httpHeaderRateLimit     = "X-Ratelimit-Limit"
	httpHeaderRateRemaining = "X-Ratelimit-Remaining"
	httpHeaderRateUsed      = "X-Ratelimit-Used"
	httpHeaderRateReset     = "X-Ratelimit-Reset"
	httpHeaderRateResource  = "X-Ratelimit-Resource"
)

// ClientLogging creates client middleware that logs request and response
// information at the given level. If the request fails without creating a
// response, it is logged with a status code of -1. The middleware uses a
// logger from the request context.
func ClientLogging(lvl zerolog.Level, opts ...ClientLoggingOption) ClientMiddleware {
	var options clientLoggingOptions
	for _, opt := range opts {
		opt(&options)
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			var err error
			var reqBody, resBody []byte

			if requestMatches(r, options.RequestBodyPatterns) {
				if r, reqBody, err = mirrorRequestBody(r); err != nil {
					return nil, err
				}
			}

			start := time.Now()
			res, err := next.RoundTrip(r)
			elapsed := time.Now().Sub(start)

			evt := zerolog.Ctx(r.Context()).
				WithLevel(lvl).
				Str("method", r.Method).
				Str("path", r.URL.String()).
				Dur("elapsed", elapsed)

			if reqBody != nil {
				evt.Bytes("request_body", reqBody)
			}

			if res != nil {
				cached := res.Header.Get(httpcache.XFromCache) != ""
				evt.Bool("cached", cached).
					Int("status", res.StatusCode)

				size := res.ContentLength
				if requestMatches(r, options.ResponseBodyPatterns) {
					if res, resBody, err = mirrorResponseBody(res); err != nil {
						return res, err
					}
					if size < 0 {
						size = int64(len(resBody))
					}
					evt.Int64("size", size).Bytes("response_body", resBody)
				} else {
					evt.Int64("size", size)
				}
			} else {
				evt.Bool("cached", false).
					Int("status", -1).
					Int64("size", -1)
			}

			addRateLimitInformationToLog(options.LogRateLimitInformation, evt, res)
			evt.Msg("github_request")
			return res, err
		})
	}
}

// ClientLoggingOption controls behavior of client request logs.
type ClientLoggingOption func(*clientLoggingOptions)

type clientLoggingOptions struct {
	RequestBodyPatterns  []*regexp.Regexp
	ResponseBodyPatterns []*regexp.Regexp

	// Output control
	LogRateLimitInformation *RateLimitLoggingOption
}

// RateLimitLoggingOption controls which rate limit information is logged.
type RateLimitLoggingOption struct {
	Limit     bool
	Remaining bool
	Used      bool
	Reset     bool
	Resource  bool
}

// LogRequestBody enables request body logging for requests to paths matching
// any of the regular expressions in patterns. It panics if any of the patterns
// is not a valid regular expression.
func LogRequestBody(patterns ...string) ClientLoggingOption {
	regexps := compileRegexps(patterns)
	return func(opts *clientLoggingOptions) {
		opts.RequestBodyPatterns = regexps
	}
}

// LogResponseBody enables response body logging for requests to paths matching
// any of the regular expressions in patterns. It panics if any of the patterns
// is not a valid regular expression.
func LogResponseBody(patterns ...string) ClientLoggingOption {
	regexps := compileRegexps(patterns)
	return func(opts *clientLoggingOptions) {
		opts.ResponseBodyPatterns = regexps
	}
}

// LogRateLimitInformation defines which rate limit information like
// the number of requests remaining in the current rate limit window is getting logged.
func LogRateLimitInformation(options *RateLimitLoggingOption) ClientLoggingOption {
	return func(opts *clientLoggingOptions) {
		opts.LogRateLimitInformation = options
	}
}

func mirrorRequestBody(r *http.Request) (*http.Request, []byte, error) {
	switch {
	case r.Body == nil || r.Body == http.NoBody:
		return r, []byte{}, nil

	case r.GetBody != nil:
		br, err := r.GetBody()
		if err != nil {
			return r, nil, err
		}
		body, err := io.ReadAll(br)
		closeBody(br)
		return r, body, err

	default:
		body, err := io.ReadAll(r.Body)
		closeBody(r.Body)
		if err != nil {
			return r, nil, err
		}
		rCopy := r.Clone(r.Context())
		rCopy.Body = io.NopCloser(bytes.NewReader(body))
		return rCopy, body, nil
	}
}

func mirrorResponseBody(res *http.Response) (*http.Response, []byte, error) {
	body, err := io.ReadAll(res.Body)
	closeBody(res.Body)
	if err != nil {
		return res, nil, err
	}

	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, body, nil
}

func compileRegexps(pats []string) []*regexp.Regexp {
	regexps := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		regexps[i] = regexp.MustCompile(p)
	}
	return regexps
}

func requestMatches(r *http.Request, pats []*regexp.Regexp) bool {
	for _, pat := range pats {
		if pat.MatchString(r.URL.Path) {
			return true
		}
	}
	return false
}

func closeBody(b io.ReadCloser) {
	_ = b.Close() // per http.Transport impl, ignoring close errors is fine
}

func addRateLimitInformationToLog(loggingOptions *RateLimitLoggingOption, evt *zerolog.Event, res *http.Response) {
	// Exit early if no rate limit information is requested
	if loggingOptions == nil {
		return
	}

	rateLimitDict := zerolog.Dict()
	if limitHeader := res.Header.Get(httpHeaderRateLimit); loggingOptions.Limit && limitHeader != "" {
		limit, _ := strconv.Atoi(limitHeader)
		rateLimitDict.Int("limit", limit)
	}
	if remainingHeader := res.Header.Get(httpHeaderRateRemaining); loggingOptions.Remaining && remainingHeader != "" {
		remaining, _ := strconv.Atoi(remainingHeader)
		rateLimitDict.Int("remaining", remaining)
	}
	if usedHeader := res.Header.Get(httpHeaderRateUsed); loggingOptions.Used && usedHeader != "" {
		used, _ := strconv.Atoi(usedHeader)
		rateLimitDict.Int("used", used)
	}
	if resetHeader := res.Header.Get(httpHeaderRateReset); loggingOptions.Reset && resetHeader != "" {
		if v, _ := strconv.ParseInt(resetHeader, 10, 64); v != 0 {
			rateLimitDict.Time("reset", time.Unix(v, 0))
		}
	}
	if resourceHeader := res.Header.Get(httpHeaderRateResource); loggingOptions.Resource && resourceHeader != "" {
		rateLimitDict.Str("resource", resourceHeader)
	}

	evt.Dict("ratelimit", rateLimitDict)
}
