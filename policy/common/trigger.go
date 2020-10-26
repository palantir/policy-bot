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
	"fmt"
	"strings"
)

// Trigger is a flag set that marks the types of GitHub events that could
// change the value of a predicate or evaluation. It is used to optimize
// evaluation by skipping unnecessary work.
type Trigger uint32

const (
	TriggerCommit Trigger = 1 << iota
	TriggerComment
	TriggerReview
	TriggerLabel
	TriggerStatus
	TriggerPullRequest

	// TriggerStatic is a name for the empty trigger set and means the
	// computation never needs updating.
	TriggerStatic Trigger = 0

	// TriggerAll is a name for the full trigger set and means the computation
	// should update after any changes to the pull request.
	TriggerAll Trigger = TriggerCommit | TriggerComment | TriggerReview | TriggerLabel | TriggerStatus | TriggerPullRequest
)

// this is a slice instead of a map so the flags are always in a fixed order
var triggerStrings = []struct {
	T Trigger
	S string
}{
	{TriggerCommit, "Commit"},
	{TriggerComment, "Comment"},
	{TriggerReview, "Review"},
	{TriggerLabel, "Label"},
	{TriggerStatus, "Status"},
	{TriggerPullRequest, "PullRequest"},
}

// Matches returns true if flag contains any of the flags of this trigger.
func (t Trigger) Matches(flags Trigger) bool {
	return t&flags > 0
}

func (t Trigger) String() string {
	if t == TriggerStatic {
		return "Trigger(0x0=Static)"
	}

	var s strings.Builder
	fmt.Fprintf(&s, "Trigger(0x%x", uint32(t))

	i := 0
	for _, ts := range triggerStrings {
		if t.Matches(ts.T) {
			if i == 0 {
				s.WriteString("=")
			} else {
				s.WriteString("|")
			}
			s.WriteString(ts.S)
			i++
		}
	}
	s.WriteString(")")
	return s.String()
}

// Triggered defines the Trigger method, which returns a set of conditions when
// the implementor should be updated or evaluated.
type Triggered interface {
	Trigger() Trigger
}
