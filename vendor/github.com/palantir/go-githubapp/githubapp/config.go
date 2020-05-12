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
	"os"
	"strconv"
)

type Config struct {
	WebURL   string `yaml:"web_url" json:"webUrl"`
	V3APIURL string `yaml:"v3_api_url" json:"v3ApiUrl"`
	V4APIURL string `yaml:"v4_api_url" json:"v4ApiUrl"`

	App struct {
		IntegrationID int64  `yaml:"integration_id" json:"integrationId"`
		WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
		PrivateKey    string `yaml:"private_key" json:"privateKey"`
	} `yaml:"app" json:"app"`

	OAuth struct {
		ClientID     string `yaml:"client_id" json:"clientId"`
		ClientSecret string `yaml:"client_secret" json:"clientSecret"`
	} `yaml:"oauth" json:"oauth"`
}

// SetValuesFromEnv sets values in the configuration from coresponding
// environment variables, if they exist. The optional prefix is added to the
// start of the environment variable names.
func (c *Config) SetValuesFromEnv(prefix string) {
	setStringFromEnv("GITHUB_WEB_URL", prefix, &c.WebURL)
	setStringFromEnv("GITHUB_V3_API_URL", prefix, &c.V3APIURL)
	setStringFromEnv("GITHUB_V4_API_URL", prefix, &c.V4APIURL)

	setIntFromEnv("GITHUB_APP_INTEGRATION_ID", prefix, &c.App.IntegrationID)
	setStringFromEnv("GITHUB_APP_WEBHOOK_SECRET", prefix, &c.App.WebhookSecret)
	setStringFromEnv("GITHUB_APP_PRIVATE_KEY", prefix, &c.App.PrivateKey)

	setStringFromEnv("GITHUB_OAUTH_CLIENT_ID", prefix, &c.OAuth.ClientID)
	setStringFromEnv("GITHUB_OAUTH_CLIENT_SECRET", prefix, &c.OAuth.ClientSecret)
}

func setStringFromEnv(key, prefix string, value *string) {
	if v, ok := os.LookupEnv(prefix + key); ok {
		*value = v
	}
}

func setIntFromEnv(key, prefix string, value *int64) {
	if v, ok := os.LookupEnv(prefix + key); ok {
		if i, err := strconv.ParseInt(v, 10, 0); err == nil {
			*value = i
		}
	}
}
