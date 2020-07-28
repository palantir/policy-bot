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
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-baseapp/baseapp/datadog"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/palantir/policy-bot/server/handler"
)

type Config struct {
	Server   baseapp.HTTPConfig            `yaml:"server"`
	Logging  LoggingConfig                 `yaml:"logging"`
	Cache    CachingConfig                 `yaml:"cache"`
	Github   githubapp.Config              `yaml:"github"`
	Sessions SessionsConfig                `yaml:"sessions"`
	Options  handler.PullEvaluationOptions `yaml:"options"`
	Files    handler.FilesConfig           `yaml:"files"`
	Datadog  datadog.Config                `yaml:"datadog"`
	Workers  WorkerConfig                  `yaml:"workers"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`
	Text  bool   `yaml:"text" json:"text"`
}

type CachingConfig struct {
	MaxSize datasize.ByteSize `yaml:"max_size"`
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

func ParseConfig(bytes []byte) (*Config, error) {
	var c Config
	if err := yaml.UnmarshalStrict(bytes, &c); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling yaml")
	}

	c.Options.FillDefaults()
	c.Server.SetValuesFromEnv("")
	c.Github.SetValuesFromEnv("")

	if v, ok := os.LookupEnv("POLICYBOT_SESSIONS_KEY"); ok {
		c.Sessions.Key = v
	}

	return &c, nil
}
