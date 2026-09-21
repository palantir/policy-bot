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

type ChangedFilesCount struct {
	Total    ComparisonExpr              `yaml:"total,omitempty"`
	Added    ComparisonExpr              `yaml:"added,omitempty"`
	Modified ComparisonExpr              `yaml:"modified,omitempty"`
	Deleted  ComparisonExpr              `yaml:"deleted,omitempty"`
	Files    ChangedFilesCountFileFilter `yaml:"files,omitempty"`
}

type ChangedFilesCountFileFilter struct {
	Include []common.Regexp `yaml:"include,omitempty"`
	Exclude []common.Regexp `yaml:"exclude,omitempty"`
}

func (ff ChangedFilesCountFileFilter) IsZero() bool {
	return len(ff.Include) == 0 && len(ff.Exclude) == 0
}

func (ff ChangedFilesCountFileFilter) MatchesFile(filename string) bool {
	if len(ff.Exclude) > 0 && anyMatches(ff.Exclude, filename) {
		return false
	}
	if len(ff.Include) > 0 && !anyMatches(ff.Include, filename) {
		return false
	}
	return true
}

var _ Predicate = &ChangedFilesCount{}

func (pred *ChangedFilesCount) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	predicateResult := common.PredicateResult{
		ValuePhrase:     "changed files",
		ConditionPhrase: "meet",
		ConditionsMap:   make(map[string][]string),
	}

	files, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	// Count files by status
	var totalCount, addedCount, modifiedCount, deletedCount int64
	for _, f := range files {
		if !pred.Files.MatchesFile(f.Filename) {
			continue
		}

		totalCount++
		switch f.Status {
		case pull.FileAdded:
			addedCount++
		case pull.FileModified:
			modifiedCount++
		case pull.FileDeleted:
			deletedCount++
		}
	}

	// Add file filter information if specified
	if len(pred.Files.Include) > 0 {
		predicateResult.ConditionsMap["in files matching"] = getPathStrings(pred.Files.Include)
	}
	if len(pred.Files.Exclude) > 0 {
		predicateResult.ConditionsMap["excluding files matching"] = getPathStrings(pred.Files.Exclude)
	}

	const conditionKey = "the file count conditions"

	// Check total condition
	if !pred.Total.IsEmpty() {
		value := fmt.Sprintf("total %d", totalCount)
		cond := fmt.Sprintf("total files %s", pred.Total.String())

		if pred.Total.Evaluate(totalCount) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	// Check added condition
	if !pred.Added.IsEmpty() {
		value := fmt.Sprintf("added %d", addedCount)
		cond := fmt.Sprintf("added files %s", pred.Added.String())

		if pred.Added.Evaluate(addedCount) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	// Check modified condition
	if !pred.Modified.IsEmpty() {
		value := fmt.Sprintf("modified %d", modifiedCount)
		cond := fmt.Sprintf("modified files %s", pred.Modified.String())

		if pred.Modified.Evaluate(modifiedCount) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	// Check deleted condition
	if !pred.Deleted.IsEmpty() {
		value := fmt.Sprintf("deleted %d", deletedCount)
		cond := fmt.Sprintf("deleted files %s", pred.Deleted.String())

		if pred.Deleted.Evaluate(deletedCount) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	return &predicateResult, nil
}

func (pred *ChangedFilesCount) Trigger() common.Trigger {
	return common.TriggerCommit
}
