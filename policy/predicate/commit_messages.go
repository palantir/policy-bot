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

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CommitMessages struct {
	Mode       string          `yaml:"mode,omitempty"`
	Scope      string          `yaml:"scope,omitempty"`
	Matches    []common.Regexp `yaml:"matches,omitempty"`
	NotMatches []common.Regexp `yaml:"not_matches,omitempty"`
}

var _ Predicate = &CommitMessages{}

func (pred *CommitMessages) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commits")
	}

	// Default mode is "all"
	mode := pred.Mode
	if mode == "" {
		mode = "all"
	}

	// Default scope is "subject"
	scope := pred.Scope
	if scope == "" {
		scope = "subject"
	}

	// Validate mode
	if mode != "all" && mode != "any" {
		return nil, errors.Errorf("invalid mode %q: must be 'all' or 'any'", mode)
	}

	// Validate scope
	if scope != "subject" && scope != "body" && scope != "full" {
		return nil, errors.Errorf("invalid scope %q: must be 'subject', 'body', or 'full'", scope)
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "commit messages",
		ConditionPhrase: "match",
		ConditionsMap:   make(map[string][]string),
	}

	// Add pattern information
	if len(pred.Matches) > 0 {
		predicateResult.ConditionsMap["matches patterns"] = getPatternStrings(pred.Matches)
	}
	if len(pred.NotMatches) > 0 {
		predicateResult.ConditionsMap["does not match patterns"] = getPatternStrings(pred.NotMatches)
	}

	// Add mode and scope information
	predicateResult.ConditionsMap["evaluation mode"] = []string{mode}
	predicateResult.ConditionsMap["message scope"] = []string{scope}

	if len(commits) == 0 {
		predicateResult.Satisfied = false
		predicateResult.Description = "No commits found in pull request"
		return &predicateResult, nil
	}

	// Collect commit messages
	commitMessages := make([]string, 0, len(commits))
	for _, c := range commits {
		msg := pred.getCommitMessage(c, scope)
		shaShort := c.SHA
		if len(shaShort) > 7 {
			shaShort = shaShort[:7]
		}
		commitMessages = append(commitMessages, fmt.Sprintf("%s: %s", shaShort, msg))
	}
	predicateResult.Values = commitMessages

	// Evaluate based on mode
	if mode == "all" {
		// All commits must satisfy the conditions
		for _, c := range commits {
			msg := pred.getCommitMessage(c, scope)
			if !pred.messageMatches(msg) {
				shaShort := c.SHA
				if len(shaShort) > 7 {
					shaShort = shaShort[:7]
				}
				predicateResult.Satisfied = false
				predicateResult.Description = fmt.Sprintf("Commit %s does not match the required patterns", shaShort)
				return &predicateResult, nil
			}
		}
		predicateResult.Satisfied = true
		predicateResult.Description = fmt.Sprintf("All %d commits match the required patterns", len(commits))
		return &predicateResult, nil
	}

	// mode == "any": At least one commit must satisfy the conditions
	for _, c := range commits {
		msg := pred.getCommitMessage(c, scope)
		if pred.messageMatches(msg) {
			shaShort := c.SHA
			if len(shaShort) > 7 {
				shaShort = shaShort[:7]
			}
			predicateResult.Satisfied = true
			predicateResult.Description = fmt.Sprintf("Commit %s matches the required patterns", shaShort)
			return &predicateResult, nil
		}
	}
	predicateResult.Satisfied = false
	predicateResult.Description = "No commits match the required patterns"
	return &predicateResult, nil
}

func (pred *CommitMessages) getCommitMessage(c *pull.Commit, scope string) string {
	switch scope {
	case "subject":
		return c.MessageHeadline
	case "body":
		return c.MessageBody
	case "full":
		if c.MessageBody == "" {
			return c.MessageHeadline
		}
		return c.MessageHeadline + "\n\n" + c.MessageBody
	default:
		return c.MessageHeadline
	}
}

func (pred *CommitMessages) messageMatches(msg string) bool {
	// If there are Matches patterns, at least one must match
	if len(pred.Matches) > 0 {
		if !anyMatches(pred.Matches, msg) {
			return false
		}
	}

	// If there are NotMatches patterns, none must match
	if len(pred.NotMatches) > 0 {
		if anyMatches(pred.NotMatches, msg) {
			return false
		}
	}

	// If we have patterns and passed all checks, it's a match
	// If we have no patterns at all, consider it a match (vacuous truth)
	return true
}

func (pred *CommitMessages) Trigger() common.Trigger {
	return common.TriggerCommit
}

func getPatternStrings(patterns []common.Regexp) []string {
	var strs []string
	for _, r := range patterns {
		strs = append(strs, r.String())
	}
	return strs
}
