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
	"io/fs"
	"os"

	"github.com/palantir/policy-bot/policy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var validateCmdConfig struct {
	Path string
}

var ValidationCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validates the syntax and structure of a policy file.",
	Long:  "Validates the YAML syntax and logical structure of a local policy file. It does not support remote policy references.",

	RunE: validationCmd,
}

func validationCmd(cmd *cobra.Command, args []string) error {
	policyData, err := os.ReadFile(validateCmdConfig.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.Errorf("policy file does not exist: %s", validateCmdConfig.Path)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to read policy file: %s", validateCmdConfig.Path)
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
	RootCmd.AddCommand(ValidationCmd)

	ValidationCmd.Flags().StringVarP(&validateCmdConfig.Path, "policy", "p", ".policy.yml", "path to the policy file to validate")
}
