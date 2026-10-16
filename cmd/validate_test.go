// Copyright 2026 Palantir Technologies, Inc.
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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateStandaloneSlots(t *testing.T) {
	originalPath := validateCmdConfig.Path
	t.Cleanup(func() { validateCmdConfig.Path = originalPath })
	validateCmdConfig.Path = filepath.Join(t.TempDir(), "policy.yml")

	for _, test := range []struct {
		name, policy string
		valid        bool
	}{
		{"filled", `
policy:
  approval: [{slot: service, required: true}]
slots:
  service: [Service review]
approval_rules:
  - name: Service review
    requires: {count: 1, users: [reviewer]}
`, true},
		{"required empty", `policy: {approval: [{slot: service, required: true}]}
slots: {service: []}`, false},
		{"malformed unused fill", `slots: {unused: [{slot: nested}]}`, false},
		{"unsupported extension", `policy: {approval_extends: null}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(validateCmdConfig.Path, []byte(test.policy), 0600))
			err := validationCmd(ValidationCmd, nil)
			if test.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
