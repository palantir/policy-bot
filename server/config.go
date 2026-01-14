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
	"os"
	"strconv"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/palantir/go-baseapp/appmetrics/emitter/datadog"
	"github.com/palantir/go-baseapp/appmetrics/emitter/prometheus"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/server/handler"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	DefaultEnvPrefix = "POLICYBOT_"
)

type Config struct {
	Server     baseapp.HTTPConfig            `yaml:"server"`
	Logging    LoggingConfig                 `yaml:"logging"`
	Cache      CachingConfig                 `yaml:"cache"`
	Github     githubapp.Config              `yaml:"github"`
	Sessions   SessionsConfig                `yaml:"sessions"`
	Options    handler.PullEvaluationOptions `yaml:"options"`
	Files      handler.FilesConfig           `yaml:"files"`
	Datadog    datadog.Config                `yaml:"datadog"`
	Prometheus prometheus.Config             `yaml:"prometheus"`
	Workers    WorkerConfig                  `yaml:"workers"`
	OTEL       OTELConfig                    `yaml:"otel"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`
	Text  bool   `yaml:"text" json:"text"`
}

func (c *LoggingConfig) SetValuesFromEnv(prefix string) {
	if v, ok := os.LookupEnv(prefix + "LOG_LEVEL"); ok {
		c.Level = v
	}
	if v, ok := os.LookupEnv(prefix + "LOG_TEXT"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Text = b
		}
	}
}

type CachingConfig struct {
	// The maximum size of the the HTTP cache associated with each GitHub
	// client. The total amount of memory used for caching is approximately
	// this value multiplied by the total number of active GitHub clients.
	MaxSize datasize.ByteSize `yaml:"max_size"`

	// The size of the global cache for commit push times. Each entry uses
	// roughly 100 bytes of memory.
	PushedAtSize int `yaml:"pushed_at_size"`

	// The size of the global cache for parsed CODEOWNERS content. This caches
	// the parsed CODEOWNERS file for a repository at a given base branch commit
	// to avoid repeated HTTP requests.
	CodeownersSize int `yaml:"codeowners_size"`
}

type WorkerConfig struct {
	Workers       int           `yaml:"workers"`
	QueueSize     int           `yaml:"queue_size"`
	GithubTimeout time.Duration `yaml:"github_timeout"`
}

type SessionsConfig struct {
	Key      string `yaml:"key"`
	Lifetime string `yaml:"lifetime"`
}

// OTELConfig configures OpenTelemetry tracing.
type OTELConfig struct {
	// Enabled controls whether OpenTelemetry tracing is active.
	Enabled bool `yaml:"enabled"`

	// ServiceName is the name used to identify this service in traces.
	// Defaults to "policy-bot" if not specified.
	ServiceName string `yaml:"service_name"`
}

func (c *OTELConfig) SetValuesFromEnv(prefix string) {
	if v, ok := os.LookupEnv(prefix + "OTEL_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Enabled = b
		}
	}
	if v, ok := os.LookupEnv("OTEL_SERVICE_NAME"); ok && c.ServiceName == "" {
		c.ServiceName = v
	}
}

func ParseConfig(bytes []byte) (*Config, error) {
	var c Config
	if err := yaml.UnmarshalStrict(bytes, &c); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling yaml")
	}

	envPrefix := DefaultEnvPrefix
	if v, ok := os.LookupEnv("POLICYBOT_ENV_PREFIX"); ok {
		envPrefix = v
	}

	c.Options.SetValuesFromEnv(envPrefix + "OPTIONS_")
	c.Server.SetValuesFromEnv(envPrefix)
	c.Logging.SetValuesFromEnv(envPrefix)
	c.Github.SetValuesFromEnv("")
	c.OTEL.SetValuesFromEnv(envPrefix)

	if v, ok := os.LookupEnv(envPrefix + "SESSIONS_KEY"); ok {
		c.Sessions.Key = v
	}

	return &c, nil
}
