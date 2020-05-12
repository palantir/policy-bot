// Copyright 2020 Palantir Technologies, Inc.
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

// Code sourced from the following and modified in a few ways:
// https://github.com/zenazn/goji/blob/a16712d37ba72246f71f9c8012974d46f8e61d16/web/mutil/writer_proxy.go

// Copyright (c) 2014, 2015, 2016 Carl Jackson (carl@avtok.com)
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package baseapp

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// RecordingResponseWriter is a proxy for an http.ResponseWriter that
// counts bytes written and http status send to the underlying ResponseWriter.
type RecordingResponseWriter interface {
	http.ResponseWriter

	// Status returns the HTTP status of the request, or 0 if one has not
	// yet been sent.
	Status() int

	// BytesWritten returns the total number of bytes sent to the client.
	BytesWritten() int64
}

func WrapWriter(w http.ResponseWriter) RecordingResponseWriter {
	_, cn := w.(http.CloseNotifier)
	_, fl := w.(http.Flusher)
	_, hj := w.(http.Hijacker)
	_, rf := w.(io.ReaderFrom)

	bp := basicRecorder{ResponseWriter: w}
	if cn && fl && hj && rf {
		return &fancyRecorder{bp}
	}
	if fl {
		return &flushRecorder{bp}
	}
	return &bp
}

type basicRecorder struct {
	http.ResponseWriter
	code         int
	bytesWritten int64
}

func (b *basicRecorder) WriteHeader(code int) {
	if b.code == 0 {
		b.code = code
	}
	b.ResponseWriter.WriteHeader(code)
}

func (b *basicRecorder) Write(buf []byte) (int, error) {
	if b.code == 0 {
		b.code = http.StatusOK
	}
	n, err := b.ResponseWriter.Write(buf)
	b.bytesWritten += int64(n)
	return n, err
}

func (b *basicRecorder) Status() int {
	return b.code
}

func (b *basicRecorder) BytesWritten() int64 {
	return b.bytesWritten
}

// fancyRecorder is a writer that additionally satisfies http.CloseNotifier,
// http.Flusher, http.Hijacker, and io.ReaderFrom. It exists for the common case
// of wrapping the http.ResponseWriter that package http gives you, in order to
// make the proxied object support the full method set of the proxied object.
type fancyRecorder struct {
	basicRecorder
}

func (f *fancyRecorder) CloseNotify() <-chan bool {
	cn := f.basicRecorder.ResponseWriter.(http.CloseNotifier)
	return cn.CloseNotify()
}
func (f *fancyRecorder) Flush() {
	fl := f.basicRecorder.ResponseWriter.(http.Flusher)
	fl.Flush()
}
func (f *fancyRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj := f.basicRecorder.ResponseWriter.(http.Hijacker)
	return hj.Hijack()
}
func (f *fancyRecorder) ReadFrom(r io.Reader) (int64, error) {
	if f.code == 0 {
		f.code = http.StatusOK
	}
	rf := f.basicRecorder.ResponseWriter.(io.ReaderFrom)
	n, err := rf.ReadFrom(r)
	f.bytesWritten += n
	return n, err
}

var _ http.CloseNotifier = &fancyRecorder{}
var _ http.Flusher = &fancyRecorder{}
var _ http.Hijacker = &fancyRecorder{}
var _ io.ReaderFrom = &fancyRecorder{}

type flushRecorder struct {
	basicRecorder
}

func (f *flushRecorder) Flush() {
	fl := f.basicRecorder.ResponseWriter.(http.Flusher)
	fl.Flush()
}

var _ http.Flusher = &flushRecorder{}
