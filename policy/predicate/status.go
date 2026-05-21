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
	"github.com/pkg/errors"
)

type AllowedConclusions []string

type HasStatus struct {
	Conclusions AllowedConclusions `yaml:"conclusions,omitempty"`
	Statuses    []string           `yaml:"statuses,omitempty"`
}

func NewHasStatus(statuses []string, conclusions []string) *HasStatus {
	return &HasStatus{
		Conclusions: conclusions,
		Statuses:    statuses,
	}
}

var _ Predicate = HasStatus{}

func (pred HasStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	statuses, err := prctx.LatestStatuses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list commit statuses")
	}

	conclusions := pred.Conclusions
	if len(conclusions) == 0 {
		conclusions = AllowedConclusions{"success"}
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:     "status checks",
		ConditionPhrase: fmt.Sprintf("exist and have conclusion %s", conclusions.joinWithOr()),
	}

	// Group failures into three buckets so the resulting description points
	// directly at the actionable problem for each required status:
	//   - missing: the check has not reported any conclusion yet
	//   - notSuccess: the check ran but ended in a conclusion that is not in
	//     the allowed set, distinguishing skipped from failure to make triage
	//     faster
	var missingStatuses []string
	var failingStatuses []string
	skippedAsNotSuccess := make(map[string][]string) // conclusion -> []status names
	for _, status := range pred.Statuses {
		result, ok := statuses[status]
		if !ok {
			missingStatuses = append(missingStatuses, status)
			continue
		}
		if !slices.Contains(conclusions, result) {
			failingStatuses = append(failingStatuses, status)
			skippedAsNotSuccess[result] = append(skippedAsNotSuccess[result], status)
		}
	}

	if len(missingStatuses) == 0 && len(failingStatuses) == 0 {
		predicateResult.Values = pred.Statuses
		predicateResult.Satisfied = true
		return &predicateResult, nil
	}

	predicateResult.Satisfied = false
	predicateResult.Description = buildHasStatusDescription(missingStatuses, skippedAsNotSuccess, conclusions)

	// Preserve historical Values semantics: callers (and the legacy details
	// template) read .Values as the failing statuses. Missing statuses are
	// listed first because they are the most common cause of "policy-bot:
	// main = ERROR" with no actionable detail at FINN.
	values := append([]string{}, missingStatuses...)
	values = append(values, failingStatuses...)
	predicateResult.Values = values
	return &predicateResult, nil
}

// buildHasStatusDescription produces a concise, deterministic description that
// names the specific status checks blocking the predicate. Output is shaped to
// fit GitHub's 140-character commit status description limit when possible;
// callers further up the pipeline are responsible for truncation if the
// concatenated list would exceed that limit.
func buildHasStatusDescription(missing []string, notSuccess map[string][]string, conclusions AllowedConclusions) string {
	var segments []string
	if len(missing) > 0 {
		slices.Sort(missing)
		segments = append(segments, "Waiting on: "+strings.Join(missing, ", "))
	}

	// Render non-success conclusions grouped by their actual conclusion so the
	// reader can see "Failing: a, b" separately from "Skipped (not in allowed
	// conclusions): c". Sort the conclusion labels so output is deterministic.
	if len(notSuccess) > 0 {
		labels := make([]string, 0, len(notSuccess))
		for c := range notSuccess {
			labels = append(labels, c)
		}
		slices.Sort(labels)

		for _, label := range labels {
			names := notSuccess[label]
			slices.Sort(names)
			segments = append(segments, formatNotSuccess(label, names, conclusions))
		}
	}

	return strings.Join(segments, "; ")
}

// formatNotSuccess returns a human-readable segment for a non-success
// conclusion. "failure" is reported as "Failing: ..." while every other
// non-success conclusion (skipped, cancelled, action_required, etc.) is
// reported with its actual label so callers can distinguish a skipped check
// from a failing one without opening the policy-bot details page.
func formatNotSuccess(conclusion string, names []string, allowed AllowedConclusions) string {
	joined := strings.Join(names, ", ")
	switch conclusion {
	case "failure":
		return "Failing: " + joined
	default:
		return fmt.Sprintf("%s (not in allowed conclusions %s): %s", strings.ToUpper(conclusion[:1])+conclusion[1:], allowed.joinWithOr(), joined)
	}
}

func (pred HasStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// HasSuccessfulStatus checks that the specified statuses have a successful
// conclusion.
//
// Deprecated: use the more flexible `HasStatus` with `conclusions: ["success"]`
// instead.
type HasSuccessfulStatus []string

var _ Predicate = HasSuccessfulStatus{}

func (pred HasSuccessfulStatus) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	return HasStatus{
		Statuses: pred,
	}.Evaluate(ctx, prctx)
}

func (pred HasSuccessfulStatus) Trigger() common.Trigger {
	return common.TriggerStatus
}

// joinWithOr returns a string that represents the allowed conclusions in a
// format that can be used in a sentence. For example, if the allowed
// conclusions are "success" and "failure", this will return "success or
// failure". If there are more than two conclusions, the first n-1 will be
// separated by commas.
func (c AllowedConclusions) joinWithOr() string {
	slices.Sort(c)

	length := len(c)
	switch length {
	case 0:
		return ""
	case 1:
		return c[0]
	case 2:
		return c[0] + " or " + c[1]
	}

	head, tail := c[:length-1], c[length-1]

	return strings.Join(head, ", ") + ", or " + tail
}
