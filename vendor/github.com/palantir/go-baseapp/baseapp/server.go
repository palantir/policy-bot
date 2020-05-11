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

package baseapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
	"goji.io"
)

// Server is the base server type. It is usually embedded in an
// application-specific struct.
type Server struct {
	config     HTTPConfig
	middleware []func(http.Handler) http.Handler
	logger     zerolog.Logger
	mux        *goji.Mux
	server     *http.Server

	registry metrics.Registry

	// functions that are called once on start
	initFns []func(*Server)
	init    sync.Once
}

// Param configures a Server instance.
type Param func(b *Server) error

// NewServer creates a Server instance from configuration and parameters.
func NewServer(c HTTPConfig, params ...Param) (*Server, error) {
	logger := zerolog.Nop()
	base := &Server{
		config:     c,
		middleware: nil,
		logger:     logger,
		mux:        goji.NewMux(),
		registry:   metrics.DefaultRegistry,
	}

	for _, p := range params {
		if err := p(base); err != nil {
			return base, err
		}
	}

	if base.middleware == nil {
		base.middleware = DefaultMiddleware(base.logger, base.registry)
	}

	for _, middleware := range base.middleware {
		base.mux.Use(middleware)
	}

	if base.server == nil {
		base.server = &http.Server{}
	}

	if base.server.Addr == "" {
		addr := c.Address + ":" + strconv.Itoa(c.Port)
		base.server.Addr = addr
	}

	if base.server.Handler == nil {
		base.server.Handler = base.mux
	}

	return base, nil
}

// HTTPConfig returns the server configuration.
func (s *Server) HTTPConfig() HTTPConfig {
	return s.config
}

// HTTPServer returns the underlying HTTP Server.
func (s *Server) HTTPServer() *http.Server {
	return s.server
}

// Mux returns the root mux for the server.
func (s *Server) Mux() *goji.Mux {
	return s.mux
}

// Logger returns the root logger for the server.
func (s *Server) Logger() zerolog.Logger {
	return s.logger
}

// Registry returns the root metrics registry for the server.
func (s *Server) Registry() metrics.Registry {
	return s.registry
}

// Start starts the server and blocks.
func (s *Server) start() error {
	s.init.Do(func() {
		for _, fn := range s.initFns {
			fn(s)
		}
	})

	addr := s.config.Address + ":" + strconv.Itoa(s.config.Port)
	s.logger.Info().Msgf("Server listening on %s", addr)

	tlsConfig := s.config.TLSConfig
	if tlsConfig != nil {
		return s.server.ListenAndServeTLS(tlsConfig.CertFile, tlsConfig.KeyFile)
	}

	return s.server.ListenAndServe()
}

// Start starts the server and blocks.
func (s *Server) Start() error {
	// maintain backwards compatibility
	if s.config.ShutdownWaitTime == nil {
		return s.start()
	}

	quit := make(chan error)
	go func() {
		if err := s.start(); err != nil {
			quit <- err
		}
	}()

	// SIGKILL and SIGSTOP cannot be caught, so don't bother adding them here
	interrupt := make(chan os.Signal, 2)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-interrupt:
		s.logger.Info().Msg("Caught interrupt, gracefully shutting down")
	case err := <-quit:
		if err != http.ErrServerClosed {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *s.config.ShutdownWaitTime)
	defer cancel()
	return errors.Wrap(s.HTTPServer().Shutdown(ctx), "Failed shutting down gracefully")
}

// WriteJSON writes a JSON response or an error if mashalling the object fails.
func WriteJSON(w http.ResponseWriter, status int, obj interface{}) {
	w.Header().Set("Content-Type", "application/json")

	b, err := json.Marshal(obj)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"error": %s}`, strconv.Quote(err.Error()))
	} else {
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
}
