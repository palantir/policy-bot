// Copyright 2020 Palantir Technologies, Inc.
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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTriggerMatches(t *testing.T) {
	tests := []struct {
		Trigger Trigger
		Flags   Trigger
		Matches bool
	}{
		{TriggerCommit, TriggerCommit, true},
		{TriggerCommit | TriggerLabel, TriggerCommit, true},
		{TriggerCommit | TriggerLabel, TriggerLabel, true},
		{TriggerAll, TriggerStatus, true},
		{TriggerStatic, TriggerCommit, false},
		{TriggerAll, TriggerStatic, false},
	}

	for _, test := range tests {
		if test.Matches {
			assert.True(t, test.Trigger.Matches(test.Flags), "expected %s to match %s", test.Trigger, test.Flags)
		} else {
			assert.False(t, test.Trigger.Matches(test.Flags), "expected %s to not match %s", test.Trigger, test.Flags)
		}
	}
}

func TestTriggerString(t *testing.T) {
	tests := []struct {
		Trigger Trigger
		String  string
	}{
		{TriggerStatic, "Trigger(0x0=Static)"},
		{TriggerCommit, "Trigger(0x1=Commit)"},
		{TriggerCommit | TriggerReview | TriggerStatus, "Trigger(0x15=Commit|Review|Status)"},
	}

	for _, test := range tests {
		assert.Equal(t, test.String, test.Trigger.String(), "trigger 0x%x formatted incorrectly", test.Trigger)
	}
}
