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

package cmd

import (
	"io/ioutil"
	"os"

	"github.com/palantir/policy-bot/policy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var policyBotPolicy struct {
	Path string
}

var PolicyBotValidationCmd = &cobra.Command{
	Use:   "validate",
	Short: "Runs validation of policy-bot policy.",
	Long:  "Runs valdation of a policy-bot policy yaml file.",

	RunE: policyBotValidationCmd,
}

func readPolicyBotPolicy(cfgFile string) ([]byte, error) {
	fi, err := os.Stat(cfgFile)
	if os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "policy file does not exist: %s", cfgFile)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed fetching policy file: %s", cfgFile)
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("policy file is not a regular file: " + cfgFile)
	}

	var bytes []byte
	bytes, err = ioutil.ReadFile(cfgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading policy file: %s", cfgFile)
	}

	return bytes, nil
}

func policyBotValidationCmd(cmd *cobra.Command, args []string) error {
	policyData, err := readPolicyBotPolicy(policyBotPolicy.Path)
	if err != nil {
		return errors.Wrapf(err, "failed to read policy file")
	}
	var policyConfig policy.Config
	if err := yaml.UnmarshalStrict(policyData, &policyConfig); err != nil {
		return errors.Wrapf(err, "failed to parse policy from yaml file")
	}
	if _, err := policy.ParsePolicy(&policyConfig, nil); err != nil {
		return errors.Wrapf(err, "failed to parse policy")
	}
	return nil
}

func init() {
	RootCmd.AddCommand(PolicyBotValidationCmd)

	PolicyBotValidationCmd.Flags().StringVarP(&policyBotPolicy.Path, "policy", "p", ".policy.yml", "policy file for policy-bot")
}
