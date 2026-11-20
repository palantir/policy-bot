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

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckRunSkipsUnassociatedEvents(t *testing.T) {
	for _, branch := range []string{"main", "feature"} {
		t.Run(branch, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"action":"completed","repository":{"default_branch":"main"},"check_run":{"head_branch":%q,"head_sha":"head","conclusion":"success","pull_requests":[]}}`, branch))
			require.NoError(t, (&CheckRun{}).Handle(context.Background(), "check_run", "delivery", payload))
		})
	}
}
