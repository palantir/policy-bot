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
	"net/url"
	"regexp"
	"strings"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/gregjones/httpcache"
	"github.com/pkg/errors"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type ClientCreator interface {
	// NewAppClient returns a new github.Client that performs app authentication for
	// the GitHub app with a specific integration ID and private key. The returned
	// client makes all calls using the application's authorization token. The
	// client gets that token by creating and signing a JWT for the application and
	// requesting a token using it. The token is cached by the client and is
	// refreshed as needed if it expires.
	//
	// Used for performing app-level operations that are not associated with a
	// specific installation.
	//
	// See the following for more information:
	//  * https://developer.github.com/apps/building-github-apps/authenticating-with-github-apps/#authenticating-as-a-github-app
	//
	// Authenticating as a GitHub App lets you do a couple of things:
	//  * You can retrieve high-level management information about your GitHub App.
	//  * You can request access tokens for an installation of the app.
	//
	// Tips for determining the arguments for this function:
	//  * the integration ID is listed as "ID" in the "About" section of the app's page
	//  * the key bytes must be a PEM-encoded PKCS1 or PKCS8 private key for the application
	NewAppClient() (*github.Client, error)

	// NewAppV4Client returns an app-authenticated v4 API client, similar to NewAppClient.
	NewAppV4Client() (*githubv4.Client, error)

	// NewInstallationClient returns a new github.Client that performs app
	// authentication for the GitHub app with the a specific integration ID, private
	// key, and the given installation ID. The returned client makes all calls using
	// the application's authorization token. The client gets that token by creating
	// and signing a JWT for the application and requesting a token using it. The
	// token is cached by the client and is refreshed as needed if it expires.
	//
	// See the following for more information:
	//  * https://developer.github.com/apps/building-github-apps/authenticating-with-github-apps/#authenticating-as-an-installation
	//
	// Authenticating as an installation of a Github App lets you perform the following:
	//  * https://developer.github.com/v3/apps/available-endpoints/
	//
	// Tips for determining the arguments for this function:
	//  * the integration ID is listed as "ID" in the "About" section of the app's page
	//  * the installation ID is the ID that is shown in the URL of https://{githubURL}/settings/installations/{#}
	//      (navigate to the "installations" page without the # and go to the app's page to see the number)
	//  * the key bytes must be a PEM-encoded PKCS1 or PKCS8 private key for the application
	NewInstallationClient(installationID int64) (*github.Client, error)

	// NewInstallationV4Client returns an installation-authenticated v4 API client, similar to NewInstallationClient.
	NewInstallationV4Client(installationID int64) (*githubv4.Client, error)

	// NewTokenClient returns a *github.Client that uses the passed in OAuth token for authentication.
	NewTokenClient(token string) (*github.Client, error)

	// NewTokenV4Client returns a *githubv4.Client that uses the passed in OAuth token for authentication.
	NewTokenV4Client(token string) (*githubv4.Client, error)
}

var (
	maxAgeRegex = regexp.MustCompile(`max-age=\d+`)
)

type key string

const installationKey = key("installationID")

// NewClientCreator returns a ClientCreator that creates a GitHub client for
// installations of the app specified by the provided arguments.
func NewClientCreator(v3BaseURL, v4BaseURL string, integrationID int, privKeyBytes []byte, opts ...ClientOption) ClientCreator {
	cc := &clientCreator{
		v3BaseURL:     v3BaseURL,
		v4BaseURL:     v4BaseURL,
		integrationID: integrationID,
		privKeyBytes:  privKeyBytes,
	}

	for _, opt := range opts {
		opt(cc)
	}

	if !strings.HasSuffix(cc.v3BaseURL, "/") {
		cc.v3BaseURL += "/"
	}

	// graphql URL should not end in trailing slash
	cc.v4BaseURL = strings.TrimSuffix(cc.v4BaseURL, "/")

	return cc
}

type clientCreator struct {
	v3BaseURL      string
	v4BaseURL      string
	integrationID  int
	privKeyBytes   []byte
	userAgent      string
	middleware     []ClientMiddleware
	cacheFunc      func() httpcache.Cache
	alwaysValidate bool
}

var _ ClientCreator = &clientCreator{}

type ClientOption func(c *clientCreator)

// ClientMiddleware modifies the transport of a GitHub client to add common
// functionality, like logging or metrics collection.
type ClientMiddleware func(http.RoundTripper) http.RoundTripper

// WithClientUserAgent sets the base user agent for all created clients.
func WithClientUserAgent(agent string) ClientOption {
	return func(c *clientCreator) {
		c.userAgent = agent
	}
}

// WithClientCaching sets an HTTP cache for all created clients
// using the provided cache implementation
// If alwaysValidate is true, the cache validates all saved responses before returning them.
// Otherwise, it respects the caching headers returned by GitHub.
// https://developer.github.com/v3/#conditional-requests
func WithClientCaching(alwaysValidate bool, cache func() httpcache.Cache) ClientOption {
	return func(c *clientCreator) {
		c.cacheFunc = cache
		c.alwaysValidate = alwaysValidate
	}
}

// WithClientMiddleware adds middleware that is applied to all created clients.
func WithClientMiddleware(middleware ...ClientMiddleware) ClientOption {
	return func(c *clientCreator) {
		c.middleware = middleware
	}
}

func (c *clientCreator) NewAppClient() (*github.Client, error) {
	base := &http.Client{Transport: http.DefaultTransport}

	installation, transportError := newAppInstallation(c.integrationID, c.privKeyBytes, c.v3BaseURL)
	middleware := append(c.middleware, installation)
	if c.cacheFunc != nil {
		middleware = append(middleware, cache(c.cacheFunc), cacheControl(c.alwaysValidate))
	}

	client, err := c.newClient(base, middleware, "application", 0)
	if err != nil {
		return nil, err
	}
	if *transportError != nil {
		return nil, *transportError
	}
	return client, nil
}

func (c *clientCreator) NewAppV4Client() (*githubv4.Client, error) {
	base := &http.Client{Transport: http.DefaultTransport}

	installation, transportError := newAppInstallation(c.integrationID, c.privKeyBytes, c.v3BaseURL)

	// The v4 API primarily uses POST requests (except for introspection queries)
	// which we cannot cache, so don't construct the middleware
	middleware := append(c.middleware, installation)

	client, err := c.newV4Client(base, middleware, "application", 0)
	if err != nil {
		return nil, err
	}
	if *transportError != nil {
		return nil, *transportError
	}
	return client, nil
}

func (c *clientCreator) NewInstallationClient(installationID int64) (*github.Client, error) {
	base := &http.Client{Transport: http.DefaultTransport}

	installation, transportError := newInstallation(c.integrationID, int(installationID), c.privKeyBytes, c.v3BaseURL)
	middleware := append(c.middleware, installation)
	if c.cacheFunc != nil {
		middleware = append(middleware, cache(c.cacheFunc), cacheControl(c.alwaysValidate))
	}

	client, err := c.newClient(base, middleware, fmt.Sprintf("installation: %d", installationID), installationID)
	if err != nil {
		return nil, err
	}
	if *transportError != nil {
		return nil, *transportError
	}
	return client, nil
}

func (c *clientCreator) NewInstallationV4Client(installationID int64) (*githubv4.Client, error) {
	base := &http.Client{Transport: http.DefaultTransport}

	installation, transportError := newInstallation(c.integrationID, int(installationID), c.privKeyBytes, c.v3BaseURL)

	// The v4 API primarily uses POST requests (except for introspection queries)
	// which we cannot cache, so don't construct the middleware
	middleware := append(c.middleware, installation)

	client, err := c.newV4Client(base, middleware, fmt.Sprintf("installation: %d", installationID), installationID)
	if err != nil {
		return nil, err
	}
	if *transportError != nil {
		return nil, *transportError
	}
	return client, nil
}

func (c *clientCreator) NewTokenClient(token string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return c.newClient(tc, c.middleware, "oauth token", 0)
}

func (c *clientCreator) NewTokenV4Client(token string) (*githubv4.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return c.newV4Client(tc, c.middleware, "oauth token", 0)
}

func (c *clientCreator) newClient(base *http.Client, middleware []ClientMiddleware, details string, installID int64) (*github.Client, error) {
	middleware = append(middleware, setInstallationID(installID))
	applyMiddleware(base, middleware)

	baseURL, err := url.Parse(c.v3BaseURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse base URL: %q", c.v3BaseURL)
	}

	client := github.NewClient(base)
	client.BaseURL = baseURL
	client.UserAgent = makeUserAgent(c.userAgent, details)

	return client, nil
}

func (c *clientCreator) newV4Client(base *http.Client, middleware []ClientMiddleware, details string, installID int64) (*githubv4.Client, error) {
	ua := makeUserAgent(c.userAgent, details)

	middleware = append([]ClientMiddleware{setUserAgentHeader(ua)}, middleware...)
	applyMiddleware(base, middleware)

	v4BaseURL, err := url.Parse(c.v4BaseURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse base URL: %q", c.v4BaseURL)
	}

	client := githubv4.NewEnterpriseClient(v4BaseURL.String(), base)
	return client, nil
}

func applyMiddleware(base *http.Client, middleware []ClientMiddleware) {
	for i := len(middleware) - 1; i >= 0; i-- {
		base.Transport = middleware[i](base.Transport)
	}
}

func newAppInstallation(integrationID int, privKeyBytes []byte, v3BaseURL string) (ClientMiddleware, *error) {
	var transportError error
	installation := func(next http.RoundTripper) http.RoundTripper {
		itr, err := ghinstallation.NewAppsTransport(next, integrationID, privKeyBytes)
		if err != nil {
			transportError = err
			return next
		}
		// leaving the v3 URL since this is used to refresh the token, not make queries
		itr.BaseURL = strings.TrimSuffix(v3BaseURL, "/")
		return itr
	}
	return installation, &transportError
}

func newInstallation(integrationID, installationID int, privKeyBytes []byte, v3BaseURL string) (ClientMiddleware, *error) {
	var transportError error
	installation := func(next http.RoundTripper) http.RoundTripper {
		itr, err := ghinstallation.New(next, integrationID, installationID, privKeyBytes)
		if err != nil {
			transportError = err
			return next
		}
		// leaving the v3 URL since this is used to refresh the token, not make queries
		itr.BaseURL = strings.TrimSuffix(v3BaseURL, "/")
		return itr
	}
	return installation, &transportError
}

func cache(cacheFunc func() httpcache.Cache) ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return &httpcache.Transport{
			Transport:           next,
			Cache:               cacheFunc(),
			MarkCachedResponses: true,
		}
	}
}

func cacheControl(alwaysValidate bool) ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		if !alwaysValidate {
			return next
		}
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			resp, err := next.RoundTrip(r)

			// Force validation to occur when the cache is disabled by setting max-age=0,
			// as the cache results will always appear as stale
			cacheControl := resp.Header.Get("Cache-Control")
			if cacheControl != "" {
				newCacheControl := maxAgeRegex.ReplaceAllString(cacheControl, "max-age=0")
				resp.Header.Set("Cache-Control", newCacheControl)
			}
			return resp, err
		})
	}
}

func makeUserAgent(base, details string) string {
	if base == "" {
		base = "github-base-app/undefined"
	}
	return fmt.Sprintf("%s (%s)", base, details)
}

func setInstallationID(installationID int64) ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			r = r.WithContext(context.WithValue(r.Context(), installationKey, installationID))
			return next.RoundTrip(r)
		})
	}
}

func setUserAgentHeader(agent string) ClientMiddleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			r.Header.Set("User-Agent", agent)
			return next.RoundTrip(r)
		})
	}
}
