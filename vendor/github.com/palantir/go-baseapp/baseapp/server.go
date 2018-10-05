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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
	"goji.io"
)

type Server struct {
	config     HTTPConfig
	middleware []func(http.Handler) http.Handler
	logger     zerolog.Logger
	mux        *goji.Mux

	registry    metrics.Registry
	initMetrics func()
	init        sync.Once
}

type Param func(b *Server) error

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

	return base, nil
}

func (s *Server) HTTPConfig() HTTPConfig {
	return s.config
}

func (s *Server) Mux() *goji.Mux {
	return s.mux
}

func (s *Server) Logger() zerolog.Logger {
	return s.logger
}

func (s *Server) Registry() metrics.Registry {
	return s.registry
}

// Start starts the server and is blocking
func (s *Server) Start() error {
	if s.initMetrics != nil {
		s.init.Do(s.initMetrics)
	}

	addr := s.config.Address + ":" + strconv.Itoa(s.config.Port)
	s.logger.Info().Msgf("Server listening on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

func WriteJSON(w http.ResponseWriter, status int, obj interface{}) {
	w.Header().Set("Content-Type", "application/json")

	b, err := json.Marshal(obj)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": %s}`, strconv.Quote(err.Error()))
	} else {
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
}
