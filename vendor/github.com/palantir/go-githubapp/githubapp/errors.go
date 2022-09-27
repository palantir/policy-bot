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

package githubapp

import (
	"fmt"
	"io"
	"runtime"

	"github.com/rcrowley/go-metrics"
)

const (
	MetricsKeyHandlerError = "github.handler.error"
)

var (
	// HandlerRecoverStackDepth is the max depth of stack trace to recover on a
	// handler panic.
	HandlerRecoverStackDepth = 32
)

func errorCounter(r metrics.Registry, event string) metrics.Counter {
	if r == nil {
		return metrics.NilCounter{}
	}

	key := MetricsKeyHandlerError
	if event != "" {
		key = fmt.Sprintf("%s[event:%s]", key, event)
	}
	return metrics.GetOrRegisterCounter(key, r)
}

// HandlerPanicError is an error created from a recovered handler panic.
type HandlerPanicError struct {
	value interface{}
	stack []runtime.Frame
}

// Value returns the exact value with which panic() was called.
func (e HandlerPanicError) Value() interface{} {
	return e.value
}

// StackTrace returns the stack of the panicking goroutine.
func (e HandlerPanicError) StackTrace() []runtime.Frame {
	return e.stack
}

// Format formats the error optionally including the stack trace.
//
//	%s    the error message
//	%v    the error message and the source file and line number for each stack frame
//
// Format accepts the following flags:
//
//	%+v   the error message and the function, file, and line for each stack frame
func (e HandlerPanicError) Format(s fmt.State, verb rune) {
	switch verb {
	case 's':
		_, _ = io.WriteString(s, e.Error())
	case 'v':
		_, _ = io.WriteString(s, e.Error())
		for _, f := range e.stack {
			_, _ = io.WriteString(s, "\n")
			if s.Flag('+') {
				_, _ = fmt.Fprintf(s, "%s\n\t", f.Function)
			}
			_, _ = fmt.Fprintf(s, "%s:%d", f.File, f.Line)
		}
	}
}

func (e HandlerPanicError) Error() string {
	v := e.value
	if err, ok := v.(error); ok {
		v = err.Error()
	}
	return fmt.Sprintf("panic: %v", v)
}

func getStack(skip int) []runtime.Frame {
	rpc := make([]uintptr, HandlerRecoverStackDepth)

	n := runtime.Callers(skip+2, rpc)
	frames := runtime.CallersFrames(rpc[0:n])

	var stack []runtime.Frame
	for {
		f, more := frames.Next()
		if !more {
			break
		}
		stack = append(stack, f)
	}
	return stack
}
