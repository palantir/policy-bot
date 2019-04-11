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
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/alexedwards/scs"
	"github.com/bluekeyes/hatpear"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-baseapp/baseapp/datadog"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/go-githubapp/oauth2"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"goji.io"
	"goji.io/pat"

	"github.com/palantir/policy-bot/server/handler"
	"github.com/palantir/policy-bot/version"
)

const (
	DefaultSessionLifetime = 24 * time.Hour
)

type Server struct {
	config *Config
	base   *baseapp.Server
}

// New instantiates a new Server.
// Callers must then invoke Start to run the Server.
func New(c *Config) (*Server, error) {
	logger := baseapp.NewLogger(c.Logging)

	lifetime, _ := time.ParseDuration(c.Sessions.Lifetime)
	if lifetime == 0 {
		lifetime = DefaultSessionLifetime
	}

	publicURL, err := url.Parse(c.Server.PublicURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed parse public URL")
	}

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

	userAgent := fmt.Sprintf("%s/%s", c.Options.AppName, version.GetVersion())
	cc, err := githubapp.NewDefaultCachingClientCreator(
		c.Github,
		githubapp.WithClientUserAgent(userAgent),
		githubapp.WithClientMiddleware(
			githubapp.ClientLogging(zerolog.DebugLevel),
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

	basePolicyHandler := handler.Base{
		ClientCreator: cc,
		BaseConfig:    &c.Server,
		Installations: githubapp.NewInstallationsService(appClient),

		PullOpts: &c.Options,
		ConfigFetcher: &handler.ConfigFetcher{
			PolicyPath: c.Options.PolicyPath,
		},
	}

	dispatcher := githubapp.NewDefaultEventDispatcher(c.Github,
		&handler.PullRequest{Base: basePolicyHandler},
		&handler.PullRequestReview{Base: basePolicyHandler},
		&handler.IssueComment{Base: basePolicyHandler},
		&handler.Status{Base: basePolicyHandler},
	)

	templates, err := handler.LoadTemplates(&c.Files)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load templates")
	}

	mux := base.Mux()

	// webhook route
	mux.Handle(pat.Post(githubapp.DefaultWebhookRoute), dispatcher)

	// additional API routes
	mux.Handle(pat.Get("/api/health"), handler.Health())
	mux.Handle(pat.Get(oauth2.DefaultRoute), oauth2.NewHandler(
		oauth2.GetConfig(c.Github, nil),
		oauth2.ForceTLS(forceTLS),
		oauth2.WithStore(&oauth2.SessionStateStore{
			Sessions: sessions,
		}),
		oauth2.OnLogin(handler.Login(c.Github, sessions)),
	))

	// additional client routes
	mux.Handle(pat.Get("/favicon.ico"), http.RedirectHandler("/static/img/favicon.ico", http.StatusFound))
	mux.Handle(pat.Get("/static/*"), handler.Static("/static/", &c.Files))
	mux.Handle(pat.Get("/"), hatpear.Try(&handler.Index{
		Base:         basePolicyHandler,
		GithubConfig: &c.Github,
		Templates:    templates,
	}))

	details := goji.SubMux()
	details.Use(handler.RequireLogin(sessions))
	details.Handle(pat.Get("/:owner/:repo/:number"), hatpear.Try(&handler.Details{
		Base:      basePolicyHandler,
		Sessions:  sessions,
		Templates: templates,
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
