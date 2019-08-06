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

package githubapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	DefaultWebhookRoute string = "/api/github/hook"
)

type EventHandler interface {
	// Handles returns a list of GitHub events that this handler handles
	// See https://developer.github.com/v3/activity/events/types/
	Handles() []string

	// Handle processes the GitHub event "eventType" with the given delivery ID
	// and payload. The EventDispatcher guarantees that the Handle method will
	// only be called for the events returned by Handles().
	//
	// If Handle returns an error, processing stops and the error is passed
	// directly to the configured error handler.
	Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error
}

// ErrorCallback is called when an event handler returns an error. The error
// from the handler is passed directly as the final argument.
type ErrorCallback func(w http.ResponseWriter, r *http.Request, err error)

// ResponseCallback is called to send a response to GitHub after an event is
// handled. It is passed the event type and a flag indicating if an event
// handler was called for the event.
type ResponseCallback func(w http.ResponseWriter, r *http.Request, event string, handled bool)

// DispatcherOption configures properties of an event dispatcher.
type DispatcherOption func(*eventDispatcher)

// WithErrorCallback sets the error callback for an event dispatcher.
func WithErrorCallback(onError ErrorCallback) DispatcherOption {
	return func(d *eventDispatcher) {
		if onError != nil {
			d.onError = onError
		}
	}
}

// WithResponseCallback sets the response callback for an event dispatcher.
func WithResponseCallback(onResponse ResponseCallback) DispatcherOption {
	return func(d *eventDispatcher) {
		if onResponse != nil {
			d.onResponse = onResponse
		}
	}
}

// ValidationError is passed to error callbacks when the webhook payload fails
// validation.
type ValidationError struct {
	EventType  string
	DeliveryID string
	Cause      error
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("invalid event: %v", ve.Cause)
}

type eventDispatcher struct {
	handlerMap map[string]EventHandler
	secret     string

	onError    ErrorCallback
	onResponse ResponseCallback
}

// NewDefaultEventDispatcher is a convenience method to create an event
// dispatcher from configuration using the default error and response
// callbacks.
func NewDefaultEventDispatcher(c Config, handlers ...EventHandler) http.Handler {
	return NewEventDispatcher(handlers, c.App.WebhookSecret)
}

// NewEventDispatcher creates an http.Handler that dispatches GitHub webhook
// requests to the appropriate event handlers. It validates payload integrity
// using the given secret value.
//
// Responses are controlled by optional error and response callbacks. If these
// options are not provided, default callbacks are used.
func NewEventDispatcher(handlers []EventHandler, secret string, opts ...DispatcherOption) http.Handler {
	handlerMap := make(map[string]EventHandler)

	// Iterate in reverse so the first entries in the slice have priority
	for i := len(handlers) - 1; i >= 0; i-- {
		for _, event := range handlers[i].Handles() {
			handlerMap[event] = handlers[i]
		}
	}

	d := &eventDispatcher{
		handlerMap: handlerMap,
		secret:     secret,
		onError:    DefaultErrorCallback,
		onResponse: DefaultResponseCallback,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// ServeHTTP processes a webhook request from GitHub.
func (d *eventDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// initialize context for SetResponder/GetResponder
	// we store a pointer in the context so that functions deeper in the call
	// tree can modify the value without creating a new context
	var responder func(http.ResponseWriter, *http.Request)
	ctx = context.WithValue(ctx, responderKey{}, &responder)
	r = r.WithContext(ctx)

	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	if eventType == "" {
		d.onError(w, r, ValidationError{
			EventType:  eventType,
			DeliveryID: deliveryID,
			Cause:      errors.New("missing event type"),
		})
		return
	}

	logger := zerolog.Ctx(ctx).With().
		Str(LogKeyEventType, eventType).
		Str(LogKeyDeliveryID, deliveryID).
		Logger()

	// initialize context with event logger
	ctx = logger.WithContext(ctx)
	r = r.WithContext(ctx)

	payloadBytes, err := github.ValidatePayload(r, []byte(d.secret))
	if err != nil {
		d.onError(w, r, ValidationError{
			EventType:  eventType,
			DeliveryID: deliveryID,
			Cause:      err,
		})
		return
	}

	logger.Info().Msgf("Received webhook event")

	handler, ok := d.handlerMap[eventType]
	if ok {
		if err := handler.Handle(ctx, eventType, deliveryID, payloadBytes); err != nil {
			d.onError(w, r, err)
			return
		}
	}
	d.onResponse(w, r, eventType, ok)
}

// DefaultErrorCallback logs errors and responds with a 500 status code.
func DefaultErrorCallback(w http.ResponseWriter, r *http.Request, err error) {
	logger := zerolog.Ctx(r.Context())

	if ve, ok := err.(ValidationError); ok {
		logger.Warn().Err(ve.Cause).Msgf("Received invalid webhook headers or payload")
		http.Error(w, "Invalid webhook headers or payload", http.StatusBadRequest)
		return
	}

	logger.Error().Err(err).Msg("Unexpected error handling webhook request")
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// DefaultResponseCallback responds with a 200 OK for handled events and a 202
// Accepted status for all other events. By default, responses are empty.
// Event handlers may send custom responses by calling the SetResponder
// function before returning.
func DefaultResponseCallback(w http.ResponseWriter, r *http.Request, event string, handled bool) {
	if !handled && event != "ping" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if res := GetResponder(r.Context()); res != nil {
		res(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

type responderKey struct{}

// SetResponder sets a function that sends a response to GitHub after event
// processing completes. This function may only be called from event handler
// functions invoked by the event dispatcher.
//
// Customizing individual handler responses should be rare. Applications that
// want to modify the standard responses should consider registering a response
// callback before using this function.
func SetResponder(ctx context.Context, responder func(http.ResponseWriter, *http.Request)) {
	r, ok := ctx.Value(responderKey{}).(*func(http.ResponseWriter, *http.Request))
	if !ok || r == nil {
		panic("SetResponder() must be called from an event handler invoked by the go-githubapp event dispatcher")
	}
	*r = responder
}

// GetResponder returns the response function that was set by an event handler.
// If no response function exists, it returns nil. There is usually no reason
// to call this outside of a response callback implementation.
func GetResponder(ctx context.Context) func(http.ResponseWriter, *http.Request) {
	r, ok := ctx.Value(responderKey{}).(*func(http.ResponseWriter, *http.Request))
	if !ok || r == nil {
		return nil
	}
	return *r
}
