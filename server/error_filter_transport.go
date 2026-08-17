package server

import (
	"net/http"
)

// errorFilteringTransport wraps an http.RoundTripper and prevents 5xx error
// responses from being cached. This fixes the issue where httpcache caches
// error responses in violation of RFC 7231.
type errorFilteringTransport struct {
	transport http.RoundTripper
}

// NewErrorFilteringTransport creates a new error filtering transport that wraps
// the provided RoundTripper.
func NewErrorFilteringTransport(transport http.RoundTripper) http.RoundTripper {
	return &errorFilteringTransport{transport: transport}
}

// RoundTrip implements http.RoundTripper. It calls the wrapped transport and
// strips cache-related headers from 5xx error responses to prevent them from
// being cached.
func (t *errorFilteringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// If the response is a 5xx error, remove cache headers to prevent httpcache
	// from caching the error response. This is required by RFC 7231 section 6.6.
	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		resp.Header.Del("ETag")
		resp.Header.Del("Cache-Control")
		resp.Header.Del("Expires")
		resp.Header.Del("Last-Modified")
	}

	return resp, nil
}
