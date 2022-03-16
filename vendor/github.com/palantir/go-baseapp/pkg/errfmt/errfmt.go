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

// Package errfmt implements formatting for error types. Specifically, it prints
// error messages with the deepest available stacktrace for errors that include
// stacktraces.
package errfmt

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

type causer interface {
	Cause() error
}

type pkgErrorsStackTracer interface {
	StackTrace() errors.StackTrace
}

type runtimeStackTracer interface {
	StackTrace() []runtime.Frame
}

// Print returns a string representation of err. It returns the empty string if
// err is nil.
func Print(err error) string {
	if err == nil {
		return ""
	}

	var deepestStack interface{}
	currErr := err
	for currErr != nil {
		switch currErr.(type) {
		case pkgErrorsStackTracer, runtimeStackTracer:
			deepestStack = currErr
		}

		cause, ok := currErr.(causer)
		if !ok {
			break
		}
		currErr = cause.Cause()
	}

	return err.Error() + fmtStack(deepestStack)
}

func fmtStack(tracer interface{}) string {
	switch t := tracer.(type) {
	case pkgErrorsStackTracer:
		return fmt.Sprintf("%+v", t.StackTrace())
	case runtimeStackTracer:
		var s strings.Builder
		for _, frame := range t.StackTrace() {
			s.WriteByte('\n')
			_, _ = fmt.Fprintf(&s, "%s\n\t", frame.Function)
			_, _ = fmt.Fprintf(&s, "%s:%d", frame.File, frame.Line)
		}
		return s.String()
	default:
		return ""
	}
}
