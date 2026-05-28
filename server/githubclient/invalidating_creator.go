// Copyright 2025 Palantir Technologies, Inc.
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

package githubclient

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type ctxKey struct{ name string }

var installationIDCtxKey = ctxKey{"installationID"}

// ContextWithInstallationID returns a new context carrying the installation ID.
func ContextWithInstallationID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, installationIDCtxKey, id)
}

// InstallationIDFromContext retrieves the installation ID stored by ContextWithInstallationID.
func InstallationIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(installationIDCtxKey).(int64)
	return id, ok
}

// SetInstallationIDMiddleware returns a ClientMiddleware that stamps each outgoing
// request's context with the given installationID so retry logic can read it back.
func SetInstallationIDMiddleware(installationID int64) githubapp.ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			r = r.WithContext(ContextWithInstallationID(r.Context(), installationID))
			return next.RoundTrip(r)
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// InvalidatingClientCreator wraps an un-cached githubapp.ClientCreator and adds
// a controlled per-installation cache. Call Invalidate to evict a cached client
// so the next call builds a fresh ghinstallation.Transport (and fetches a new token).
type InvalidatingClientCreator struct {
	cfg        githubapp.Config
	middleware []githubapp.ClientMiddleware
	otherOpts  []githubapp.ClientOption

	// delegate handles non-installation methods (app client, token clients).
	delegate githubapp.ClientCreator

	mu      sync.Mutex
	v3Cache map[int64]*github.Client
	v4Cache map[int64]*githubv4.Client
}

var _ githubapp.ClientCreator = (*InvalidatingClientCreator)(nil)

// NewInvalidatingClientCreator builds an un-cached delegate and wraps it. The
// retry-on-401 middleware is automatically appended to middleware as the outermost
// layer so every installation client recovers from transient GitHub auth failures
// without the caller having to wire it explicitly. Pass non-middleware client
// options (UserAgent, Timeout, Caching, ...) via opts.
func NewInvalidatingClientCreator(c githubapp.Config, middleware []githubapp.ClientMiddleware, opts ...githubapp.ClientOption) (*InvalidatingClientCreator, error) {
	if c.V3APIURL == "" || c.V4APIURL == "" {
		return nil, errors.New("githubclient: V3APIURL and V4APIURL must be set")
	}

	icc := &InvalidatingClientCreator{
		cfg:       c,
		otherOpts: opts,
		v3Cache:   make(map[int64]*github.Client),
		v4Cache:   make(map[int64]*githubv4.Client),
	}

	// Append the retry middleware as the outermost layer. It closes over icc,
	// which is fully allocated by now; no late binding required.
	icc.middleware = append(append([]githubapp.ClientMiddleware{}, middleware...), NewRetryOn401Middleware(icc))

	allOpts := append(append([]githubapp.ClientOption{}, opts...), githubapp.WithClientMiddleware(icc.middleware...))
	icc.delegate = githubapp.NewClientCreator(
		c.V3APIURL,
		c.V4APIURL,
		c.App.IntegrationID,
		[]byte(c.App.PrivateKey),
		allOpts...,
	)
	return icc, nil
}

// Invalidate evicts both the v3 and v4 cached clients for the given installation.
// Safe to call when no entry exists.
func (c *InvalidatingClientCreator) Invalidate(installationID int64) {
	c.mu.Lock()
	delete(c.v3Cache, installationID)
	delete(c.v4Cache, installationID)
	c.mu.Unlock()
}

// perInstallationDelegate builds a delegate ClientCreator whose middleware
// stamps every outgoing request with installationID (innermost), then runs the
// caller's middleware (logging, metrics, tracing), then the retry middleware
// (outermost) which observes response status codes.
func (c *InvalidatingClientCreator) perInstallationDelegate(installationID int64) githubapp.ClientCreator {
	mw := make([]githubapp.ClientMiddleware, 0, len(c.middleware)+1)
	mw = append(mw, SetInstallationIDMiddleware(installationID))
	mw = append(mw, c.middleware...)
	opts := append(append([]githubapp.ClientOption{}, c.otherOpts...), githubapp.WithClientMiddleware(mw...))
	return githubapp.NewClientCreator(
		c.cfg.V3APIURL,
		c.cfg.V4APIURL,
		c.cfg.App.IntegrationID,
		[]byte(c.cfg.App.PrivateKey),
		opts...,
	)
}

func (c *InvalidatingClientCreator) NewInstallationClient(installationID int64) (*github.Client, error) {
	c.mu.Lock()
	if client, ok := c.v3Cache[installationID]; ok {
		c.mu.Unlock()
		return client, nil
	}
	c.mu.Unlock()

	client, err := c.perInstallationDelegate(installationID).NewInstallationClient(installationID)
	if err != nil {
		return nil, errors.Wrapf(err, "githubclient: build v3 client for installation %d", installationID)
	}

	c.mu.Lock()
	if existing, ok := c.v3Cache[installationID]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	c.v3Cache[installationID] = client
	c.mu.Unlock()

	return client, nil
}

func (c *InvalidatingClientCreator) NewInstallationV4Client(installationID int64) (*githubv4.Client, error) {
	c.mu.Lock()
	if client, ok := c.v4Cache[installationID]; ok {
		c.mu.Unlock()
		return client, nil
	}
	c.mu.Unlock()

	client, err := c.perInstallationDelegate(installationID).NewInstallationV4Client(installationID)
	if err != nil {
		return nil, errors.Wrapf(err, "githubclient: build v4 client for installation %d", installationID)
	}

	c.mu.Lock()
	if existing, ok := c.v4Cache[installationID]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	c.v4Cache[installationID] = client
	c.mu.Unlock()

	return client, nil
}

func (c *InvalidatingClientCreator) NewAppClient() (*github.Client, error) {
	return c.delegate.NewAppClient()
}

func (c *InvalidatingClientCreator) NewAppV4Client() (*githubv4.Client, error) {
	return c.delegate.NewAppV4Client()
}

func (c *InvalidatingClientCreator) NewTokenClient(token string) (*github.Client, error) {
	return c.delegate.NewTokenClient(token)
}

func (c *InvalidatingClientCreator) NewTokenV4Client(token string) (*githubv4.Client, error) {
	return c.delegate.NewTokenV4Client(token)
}

func (c *InvalidatingClientCreator) NewTokenSourceClient(ts oauth2.TokenSource) (*github.Client, error) {
	return c.delegate.NewTokenSourceClient(ts)
}

func (c *InvalidatingClientCreator) NewTokenSourceV4Client(ts oauth2.TokenSource) (*githubv4.Client, error) {
	return c.delegate.NewTokenSourceV4Client(ts)
}
