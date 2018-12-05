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

package pull

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type RequestMatcher interface {
	Matches(r *http.Request, body []byte) bool
}

type ExactPathMatcher string

func (m ExactPathMatcher) Matches(r *http.Request, body []byte) bool {
	return r.URL.Path == string(m)
}

type Rule struct {
	Matcher RequestMatcher
	Count   int

	responses []SavedResponse
	err       error
}

type ResponsePlayer struct {
	Rules []*Rule
}

func (rp *ResponsePlayer) AddRule(matcher RequestMatcher, file string) *Rule {
	rule := &Rule{Matcher: matcher}
	rp.Rules = append(rp.Rules, rule)

	d, err := ioutil.ReadFile(file)
	if err != nil {
		rule.err = errors.Wrapf(err, "failed to read response file: %s", file)
		return rule
	}

	if err := yaml.Unmarshal(d, &rule.responses); err != nil {
		rule.err = errors.Wrapf(err, "failed to unmarshal response file: %s", file)
		return rule
	}

	return rule
}

type SavedResponse struct {
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

func (r *SavedResponse) Response(req *http.Request) *http.Response {
	header := make(http.Header)
	for k, v := range r.Headers {
		header.Add(k, v)
	}

	body := strings.NewReader(r.Body)

	return &http.Response{
		Status:     http.StatusText(r.Status),
		StatusCode: r.Status,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,

		Header:        header,
		Body:          ioutil.NopCloser(body),
		ContentLength: body.Size(),

		Request: req,
	}
}

func (rp *ResponsePlayer) findMatch(req *http.Request) *Rule {
	var body []byte
	if req.Body != nil {
		body, _ = ioutil.ReadAll(req.Body)
		_ = req.Body.Close()
	}

	for _, rule := range rp.Rules {
		if rule.Matcher.Matches(req, body) {
			return rule
		}
	}
	return nil
}

func (rp *ResponsePlayer) RoundTrip(req *http.Request) (*http.Response, error) {
	rule := rp.findMatch(req)
	if rule == nil {
		return errorResponse(req, http.StatusNotFound, fmt.Sprintf("no matching rule for \"%s %s\"", req.Method, req.URL.Path))
	}

	// report any error encountered during loading
	if rule.err != nil {
		return nil, rule.err
	}

	// fail if there are no responses
	if len(rule.responses) == 0 {
		return errorResponse(req, http.StatusNotFound, fmt.Sprintf("no responses for \"%s %s\"", req.Method, req.URL.Path))
	}

	index := rule.Count % len(rule.responses)
	rule.Count++

	return rule.responses[index].Response(req), nil
}

func errorResponse(req *http.Request, code int, msg string) (*http.Response, error) {
	body := strings.NewReader(msg)

	return &http.Response{
		Status:     http.StatusText(code),
		StatusCode: code,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,

		Header:        make(http.Header),
		Body:          ioutil.NopCloser(body),
		ContentLength: body.Size(),

		Request: req,
	}, nil
}
