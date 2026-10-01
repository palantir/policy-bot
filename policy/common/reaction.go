// Copyright 2025 Palantir Technologies, Inc.
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
	"slices"
	"strings"

	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// ReactionContent is one of the content values GitHub accepts for a reaction.
//
// It is validated while parsing the policy so that a misspelled reaction is a
// configuration error instead of a rule that silently never matches.
type ReactionContent string

func (c *ReactionContent) UnmarshalYAML(unmarshal func(any) error) error {
	var content string
	if err := unmarshal(&content); err != nil {
		return err
	}
	return c.set(content)
}

func (c *ReactionContent) set(content string) error {
	if !slices.Contains(pull.ReactionContents, content) {
		return errors.Errorf(
			"invalid reaction %q: must be one of %s (GitHub's reaction content values, not emoji or emoji codes)",
			content, strings.Join(pull.ReactionContents, ", "),
		)
	}
	*c = ReactionContent(content)
	return nil
}
