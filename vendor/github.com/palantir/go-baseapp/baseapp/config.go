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
	"strconv"
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

// SetValuesFromEnv sets values in the configuration from corresponding
// environment variables, if they exist. The optional prefix is added to the
// start of the environment variable names.
func (c *HTTPConfig) SetValuesFromEnv(prefix string) {
	setStringFromEnv("ADDRESS", prefix, &c.Address)
	setIntFromEnv("PORT", prefix, &c.Port)
	setStringFromEnv("PUBLIC_URL", prefix, &c.PublicURL)

	var d time.Duration
	if setDurationFromEnv("SHUTDOWN_WAIT_TIME", prefix, &d) {
		c.ShutdownWaitTime = &d
	}

	var tls TLSConfig
	if c.TLSConfig != nil {
		tls = *c.TLSConfig
	}
	setStringFromEnv("TLS_CERT_FILE", prefix, &tls.CertFile)
	setStringFromEnv("TLS_KEY_FILE", prefix, &tls.KeyFile)
	if tls.CertFile != "" || tls.KeyFile != "" {
		c.TLSConfig = &tls
	}
}

// LoggingConfig contains options for logging, such as log level and textual representation.
// It is usually embedded in a larger configuration struct.
type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`

	// Pretty will make the output human-readable
	Pretty bool `yaml:"pretty" json:"pretty"`
}

// SetValuesFromEnv sets values in the configuration from corresponding
// environment variables, if they exist. The optional prefix is added to the
// start of the environment variable names.
func (c *LoggingConfig) SetValuesFromEnv(prefix string) {
	setStringFromEnv("LOG_LEVEL", prefix, &c.Level)
	setBoolFromEnv("LOG_PRETTY", prefix, &c.Pretty)
}

func setStringFromEnv(key, prefix string, value *string) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = v
		return true
	}
	return false
}

func setDurationFromEnv(key, prefix string, value *time.Duration) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			*value = d
			return true
		}
	}
	return false
}

func setIntFromEnv(key, prefix string, value *int) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			*value = i
			return true
		}
	}
	return false
}

func setBoolFromEnv(key, prefix string, value *bool) bool {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*value = b
			return true
		}
	}
	return false
}
