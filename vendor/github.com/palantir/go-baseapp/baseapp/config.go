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
	"os"
	"time"
)

type TLSConfig struct {
	CertFile string `yaml:"cert_file" json:"certFile"`
	KeyFile  string `yaml:"key_file" json:"keyFile"`
}

// HTTPConfig contains options for HTTP servers. It is usually embedded in a
// larger configuration struct.
type HTTPConfig struct {
	Address   string     `yaml:"address" json:"address"`
	Port      int        `yaml:"port" json:"port"`
	PublicURL string     `yaml:"public_url" json:"publicUrl"`
	TLSConfig *TLSConfig `yaml:"tls_config" json:"tlsConfig"`

	ShutdownWaitTime *time.Duration `yaml:"shutdown_wait_time" json:"shutdownWaitTime"`
}

// SetValuesFromEnv sets values in the configuration from coresponding
// environment variables, if they exist. The optional prefix is added to the
// start of the environment variable names.
func (c *HTTPConfig) SetValuesFromEnv(prefix string) {
	setStringFromEnv("SERVER_PUBLIC_URL", prefix, &c.PublicURL)
}

func setStringFromEnv(key, prefix string, value *string) {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = v
	}
}

// LoggingConfig contains options for logging, such as log level and textual representation.
// It is usually embedded in a larger configuration struct.
type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`

	// Pretty will make the output human-readable
	Pretty bool `yaml:"pretty" json:"pretty"`
}
