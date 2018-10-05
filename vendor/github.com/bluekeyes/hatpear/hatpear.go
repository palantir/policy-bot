// Package hatpear provides a way to aggregate errors from HTTP handlers so
// they can be processed by middleware. Errors are stored in the context of the
// current request either manually, when using standard library handler types,
// or automatically, when using this package's handler types.
//
// Using the middleware returned by the Catch function is required for this
// package to work; usage of all other functions and types is optional.
package hatpear

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
)

type contextKey int

const (
	errorKey contextKey = iota
)

// Store stores an error into the request's context. It panics if the request
// was not configured to store errors.
func Store(r *http.Request, err error) {
	errptr, ok := r.Context().Value(errorKey).(*error)
	if !ok {
		panic("hatpear: request not configured to store errors")
	}
	// check err after checking context to fail fast if unconfigured
	if err != nil {
		*errptr = err
	}
}

// Get retrieves an error from the request's context. It returns nil if the
// request was not configured to store errors.
func Get(r *http.Request) error {
	errptr, ok := r.Context().Value(errorKey).(*error)
	if !ok {
		return nil
	}
	return *errptr
}

// Middleware adds additional functionality to an existing handler.
type Middleware func(http.Handler) http.Handler

// Catch creates middleware that processes errors stored while serving a
// request. Errors are passed to the callback, which should write them to the
// response in an appropriate format. This is usually the outermost middleware
// in a chain.
func Catch(h func(w http.ResponseWriter, r *http.Request, err error)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			ctx := context.WithValue(r.Context(), errorKey, &err)

			next.ServeHTTP(w, r.WithContext(ctx))
			if err != nil {
				h(w, r, err)
			}
		})
	}
}

// Handler is a variant on http.Handler that can return an error.
type Handler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request) error
}

// HandlerFunc is a variant on http.HandlerFunc that can return an error.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	return f(w, r)
}

// Try converts a handler to a standard http.Handler, storing any error in the
// request's context.
func Try(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h.ServeHTTP(w, r)
		Store(r, err)
	})
}

var (
	// RecoverStackDepth is the max depth of stack trace to recover on panic.
	RecoverStackDepth = 32
)

// Recover creates middleware that can recover from a panic in a handler,
// storing a PanicError for future handling.
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					Store(r, PanicError{
						value: v,
						stack: stack(1),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func stack(skip int) []runtime.Frame {
	rpc := make([]uintptr, RecoverStackDepth)

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

// PanicError is an Error created from a recovered panic.
type PanicError struct {
	value interface{}
	stack []runtime.Frame
}

// Value returns the exact value with which panic() was called.
func (e PanicError) Value() interface{} {
	return e.value
}

// StackTrace returns the stack of the panicking goroutine.
func (e PanicError) StackTrace() []runtime.Frame {
	return e.stack
}

// Format formats the error optionally including the stack trace.
//
//   %s    the error message
//   %v    the error message and the source file and line number for each stack frame
//
// Format accepts the following flags:
//
//   %+v   the error message, and the function, file, and line for each stack frame
func (e PanicError) Format(s fmt.State, verb rune) {
	switch verb {
	case 's':
		io.WriteString(s, e.Error())
	case 'v':
		io.WriteString(s, e.Error())
		for _, f := range e.stack {
			io.WriteString(s, "\n")
			if s.Flag('+') {
				fmt.Fprintf(s, "%s\n\t", f.Function)
			}
			fmt.Fprintf(s, "%s:%d", f.File, f.Line)
		}
	}
}

func (e PanicError) Error() string {
	v := e.value
	if err, ok := v.(error); ok {
		v = err.Error()
	}
	return fmt.Sprintf("panic: %v", v)
}
