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

type Config struct {
	WebURL   string `yaml:"web_url" json:"webUrl"`
	V3APIURL string `yaml:"v3_api_url" json:"v3ApiUrl"`
	V4APIURL string `yaml:"v4_api_url" json:"v4ApiUrl"`

	App struct {
		IntegrationID int    `yaml:"integration_id" json:"integrationId"`
		WebhookSecret string `yaml:"webhook_secret" json:"webhookSecret"`
		PrivateKey    string `yaml:"private_key" json:"privateKey"`
	} `yaml:"app" json:"app"`

	OAuth struct {
		ClientID     string `yaml:"client_id" json:"clientId"`
		ClientSecret string `yaml:"client_secret" json:"clientSecret"`
	} `yaml:"oauth" json:"oauth"`
}
