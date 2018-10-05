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

type ResponsePlayer struct {
	// Responses maps a URL path to a file name container response objects
	Responses map[string]string

	// Requests counts the number of requests receieve on each path
	Requests map[string]int
}

func NewResponsePlayer(responses map[string]string) *ResponsePlayer {
	return &ResponsePlayer{
		Responses: responses,
		Requests:  make(map[string]int),
	}
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

func (rp *ResponsePlayer) RoundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.Path
	if req.Body != nil {
		_ = req.Body.Close()
	}

	resFile, ok := rp.Responses[path]
	if !ok {
		return errorResponse(req, http.StatusNotFound, fmt.Sprintf("no saved response for path %s", path))
	}

	d, err := ioutil.ReadFile(resFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response file")
	}

	var responses []SavedResponse
	if err := yaml.Unmarshal(d, &responses); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response file")
	}

	if len(responses) == 0 {
		return errorResponse(req, http.StatusNotFound, fmt.Sprintf("no saved response for path %s", path))
	}

	index := rp.Requests[path] % len(responses)
	rp.Requests[path]++

	return responses[index].Response(req), nil
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
