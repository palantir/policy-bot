package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestErrorFilteringTransport_Allows5xxWithoutCacheHeaders(t *testing.T) {
	req := &http.Request{}

	resp := &http.Response{
		StatusCode: 500,
		Header: http.Header{
			"ETag":           []string{"123"},
			"Cache-Control":  []string{"public"},
			"Content-Length": []string{"0"},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}

	mock := &mockRoundTripper{response: resp}
	transport := NewErrorFilteringTransport(mock)

	result, err := transport.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, 500, result.StatusCode)
	assert.Empty(t, result.Header.Get("ETag"))
	assert.Empty(t, result.Header.Get("Cache-Control"))
	assert.Empty(t, result.Header.Get("Expires"))
	assert.Empty(t, result.Header.Get("Last-Modified"))
	assert.Equal(t, "0", result.Header.Get("Content-Length"))
}

func TestErrorFilteringTransport_Allows200WithCacheHeaders(t *testing.T) {
	req := &http.Request{}

	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"ETag":          []string{"123"},
			"Cache-Control": []string{"public"},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}

	mock := &mockRoundTripper{response: resp}
	transport := NewErrorFilteringTransport(mock)

	result, err := transport.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, "123", result.Header.Get("ETag"))
	assert.Equal(t, "public", result.Header.Get("Cache-Control"))
}

func TestErrorFilteringTransport_PropagatesErrors(t *testing.T) {
	req := &http.Request{}
	expectedErr := errors.New("network error")

	mock := &mockRoundTripper{err: expectedErr}
	transport := NewErrorFilteringTransport(mock)

	_, err := transport.RoundTrip(req)

	assert.Equal(t, expectedErr, err)
}
