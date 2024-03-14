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

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/hatpear"
	"github.com/c2h5oh/datasize"
	"github.com/die-net/lrucache"
	"github.com/gregjones/httpcache"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-baseapp/baseapp/datadog"
	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/go-githubapp/oauth2"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/server/handler"
	"github.com/palantir/policy-bot/version"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"goji.io"
	"goji.io/pat"
)

const (
	DefaultSessionLifetime = 24 * time.Hour
	DefaultGitHubTimeout   = 10 * time.Second

	DefaultWebhookWorkers   = 10
	DefaultWebhookQueueSize = 100

	DefaultHTTPCacheSize     = 50 * datasize.MB
	DefaultPushedAtCacheSize = 100_000
)

type Server struct {
	config *Config
	base   *baseapp.Server
}

// New instantiates a new Server.
// Callers must then invoke Start to run the Server.
func New(c *Config) (*Server, error) {
	logger := baseapp.NewLogger(baseapp.LoggingConfig{
		Level:  c.Logging.Level,
		Pretty: c.Logging.Text,
	})

	lifetime, _ := time.ParseDuration(c.Sessions.Lifetime)
	if lifetime == 0 {
		lifetime = DefaultSessionLifetime
	}

	publicURL, err := url.Parse(c.Server.PublicURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed parse public URL")
	}
	if publicURL.Scheme == "" || publicURL.Host == "" {
		return nil, errors.Errorf("public URL must contain a scheme and a host: %s", c.Server.PublicURL)
	}

	basePath := strings.TrimSuffix(publicURL.Path, "/")
	forceTLS := publicURL.Scheme == "https"

	sessions := scs.NewCookieManager(c.Sessions.Key)
	sessions.Name("policy-bot")
	sessions.Lifetime(lifetime)
	sessions.Persist(true)
	sessions.HttpOnly(true)
	sessions.Secure(forceTLS)

	base, err := baseapp.NewServer(c.Server, baseapp.DefaultParams(logger, "policybot.")...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize base server")
	}

	maxSize := int64(DefaultHTTPCacheSize)
	if c.Cache.MaxSize != 0 {
		maxSize = int64(c.Cache.MaxSize)
	}

	githubTimeout := c.Workers.GithubTimeout
	if githubTimeout == 0 {
		githubTimeout = DefaultGitHubTimeout
	}

	v4URL, err := url.Parse(c.Github.V4APIURL)
	if err != nil {
		return nil, errors.Wrap(err, "invalid v4 API URL")
	}

	userAgent := fmt.Sprintf("policy-bot/%s", version.GetVersion())
	cc, err := githubapp.NewDefaultCachingClientCreator(
		c.Github,
		githubapp.WithClientUserAgent(userAgent),
		githubapp.WithClientTimeout(githubTimeout),
		githubapp.WithClientCaching(true, func() httpcache.Cache {
			return lrucache.New(maxSize, 0)
		}),
		githubapp.WithClientMiddleware(
			githubapp.ClientLogging(
				zerolog.DebugLevel,
				githubapp.LogRequestBody("^"+v4URL.Path+"$"),
			),
			githubapp.ClientMetrics(base.Registry()),
		),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize client creator")
	}

	appClient, err := cc.NewAppClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize Github app client")
	}

	app, _, err := appClient.Apps.Get(context.Background(), "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get configured GitHub app")
	}

	pushedAtSize := c.Cache.PushedAtSize
	if pushedAtSize == 0 {
		pushedAtSize = DefaultPushedAtCacheSize
	}

	globalCache, err := pull.NewLRUGlobalCache(pushedAtSize)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize global cache")
	}

	basePolicyHandler := handler.Base{
		ClientCreator: cc,
		BaseConfig:    &c.Server,
		Installations: githubapp.NewInstallationsService(appClient),
		GlobalCache:   globalCache,

		PullOpts: &c.Options,
		ConfigFetcher: &handler.ConfigFetcher{
			Loader: appconfig.NewLoader(
				[]string{c.Options.PolicyPath},
				appconfig.WithOwnerDefault(c.Options.SharedRepository, []string{
					c.Options.SharedPolicyPath,
				}),
			),
		},

		AppName: app.GetSlug(),
	}

	queueSize := c.Workers.QueueSize
	if queueSize < 1 {
		queueSize = DefaultWebhookQueueSize
	}

	workers := c.Workers.Workers
	if workers < 1 {
		workers = DefaultWebhookWorkers
	}

	dispatcher := githubapp.NewEventDispatcher(
		[]githubapp.EventHandler{
			&handler.Installation{Base: basePolicyHandler},
			&handler.MergeGroup{Base: basePolicyHandler},
			&handler.PullRequest{Base: basePolicyHandler},
			&handler.PullRequestReview{Base: basePolicyHandler},
			&handler.IssueComment{Base: basePolicyHandler},
			&handler.Status{Base: basePolicyHandler},
			&handler.CheckRun{Base: basePolicyHandler},
		},
		c.Github.App.WebhookSecret,
		githubapp.WithErrorCallback(githubapp.MetricsErrorCallback(base.Registry())),
		githubapp.WithScheduler(
			githubapp.QueueAsyncScheduler(
				queueSize, workers,
				githubapp.WithSchedulingMetrics(base.Registry()),
				githubapp.WithAsyncErrorCallback(githubapp.MetricsAsyncErrorCallback(base.Registry())),
			),
		),
	)

	templates, err := handler.LoadTemplates(&c.Files, basePath, c.Github.WebURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load templates")
	}

	var mux *goji.Mux
	if basePath == "" {
		mux = base.Mux()
	} else {
		mux = goji.SubMux()
		base.Mux().Handle(pat.New(basePath+"/*"), mux)
	}

	// webhook route
	mux.Handle(pat.Post(githubapp.DefaultWebhookRoute), dispatcher)

	simulateHandler := &handler.Simulate{
		Base: basePolicyHandler,
	}

	// additional API routes
	mux.Handle(pat.Get("/api/health"), handler.Health())
	mux.Handle(pat.Put("/api/validate"), handler.Validate())
	mux.Handle(pat.Post("/api/simulate/:owner/:repo/:number"), hatpear.Try(simulateHandler))
	mux.Handle(pat.Get(oauth2.DefaultRoute), oauth2.NewHandler(
		oauth2.GetConfig(c.Github, nil),
		oauth2.ForceTLS(forceTLS),
		oauth2.WithStore(&oauth2.SessionStateStore{
			Sessions: sessions,
		}),
		oauth2.OnLogin(handler.Login(c.Github, basePath, sessions)),
	))

	// additional client routes
	mux.Handle(pat.Get("/favicon.ico"), http.RedirectHandler(basePath+"/static/img/favicon.ico", http.StatusFound))
	mux.Handle(pat.Get("/static/*"), handler.Static(basePath+"/static/", &c.Files))
	mux.Handle(pat.Get("/"), hatpear.Try(&handler.Index{
		Base:         basePolicyHandler,
		GithubConfig: &c.Github,
		Templates:    templates,
	}))

	detailsHandler := handler.Details{
		Base:      basePolicyHandler,
		Sessions:  sessions,
		Templates: templates,
	}

	details := goji.SubMux()
	details.Use(handler.RequireLogin(sessions, basePath))
	details.Handle(pat.Get("/:owner/:repo/:number"), hatpear.Try(&detailsHandler))
	details.Handle(pat.Get("/:owner/:repo/:number/reviewers"), hatpear.Try(&handler.DetailsReviewers{
		Details: detailsHandler,
	}))
	mux.Handle(pat.New("/details/*"), details)

	return &Server{
		config: c,
		base:   base,
	}, nil
}

// Start is blocking and long-running
func (s *Server) Start() error {
	if s.config.Datadog.Address != "" {
		if err := datadog.StartEmitter(s.base, s.config.Datadog); err != nil {
			return err
		}
	}
	return s.base.Start()
}
