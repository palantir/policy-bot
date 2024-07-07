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

package predicate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type Predicate interface {
	common.Triggered

	// Evaluate determines if the predicate is satisfied.
	Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error)
}

type unit struct{}
type set map[string]unit

// allowedConclusions can be one of:
// action_required, cancelled, failure, neutral, success, skipped, stale,
// timed_out
type allowedConclusions set

// UnmarshalYAML implements the yaml.Unmarshaler interface for allowedConclusions.
// This allows the predicate to be specified in the input as a list of strings,
// which we then convert to a set of strings, for easy membership testing.
func (c *allowedConclusions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var conclusions []string
	if err := unmarshal(&conclusions); err != nil {
		return fmt.Errorf("failed to unmarshal conclusions: %v", err)
	}

	*c = make(allowedConclusions, len(conclusions))
	for _, conclusion := range conclusions {
		(*c)[conclusion] = unit{}
	}

	return nil
}

// joinWithOr returns a string that represents the allowed conclusions in a
// format that can be used in a sentence. For example, if the allowed
// conclusions are "success" and "failure", this will return "success or
// failure". If there are more than two conclusions, the first n-1 will be
// separated by commas.
func (c allowedConclusions) joinWithOr() string {
	length := len(c)

	keys := make([]string, 0, length)
	for key := range c {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	switch length {
	case 0:
		return ""
	case 1:
		return keys[0]
	case 2:
		return keys[0] + " or " + keys[1]
	}

	head, tail := keys[:length-1], keys[length-1]

	return strings.Join(head, ", ") + ", or " + tail
}

// If unspecified, require the status to be successful.
var defaultConclusions allowedConclusions = allowedConclusions{
	"success": unit{},
}
