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
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

func getPathStrings(re []common.Regexp, globs []common.Glob) []string {
	var paths []string
	for _, r := range re {
		paths = append(paths, r.String())
	}
	for _, g := range globs {
		paths = append(paths, string(g))
	}
	return paths
}

type ChangedFiles struct {
	Paths       []common.Regexp `yaml:"paths,omitempty"`
	Globs       []common.Glob   `yaml:"globs,omitempty"`
	IgnorePaths []common.Regexp `yaml:"ignore,omitempty"`
	IgnoreGlobs []common.Glob   `yaml:"ignore_globs,omitempty"`
}

var _ Predicate = &ChangedFiles{}

func (pred *ChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)
	ignorePaths := getPathStrings(pred.IgnorePaths, pred.IgnoreGlobs)

	predicateResult := common.PredicateResult{
		ValuePhrase:     "changed files",
		ConditionPhrase: "match",
		ConditionsMap: map[string][]string{
			"path patterns":  paths,
			"while ignoring": ignorePaths,
		},
	}

	files, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	changedFiles := []string{}

	for _, f := range files {

		changedFiles = append(changedFiles, f.Filename)

		if anyMatches(pred.IgnorePaths, f.Filename) || anyGlobMatches(pred.IgnoreGlobs, f.Filename) {
			continue
		}

		if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
			predicateResult.Values = []string{f.Filename}
			predicateResult.Description = f.Filename + " was changed"
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}

	predicateResult.Values = changedFiles
	predicateResult.Description = "No changed files match the required patterns"
	predicateResult.Satisfied = false
	return &predicateResult, nil
}

func (pred *ChangedFiles) Trigger() common.Trigger {
	return common.TriggerCommit
}

type OnlyChangedFiles struct {
	Paths []common.Regexp `yaml:"paths,omitempty"`
	Globs []common.Glob   `yaml:"globs,omitempty"`
}

var _ Predicate = &OnlyChangedFiles{}

func (pred *OnlyChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)

	predicateResult := common.PredicateResult{
		ValuePhrase:     "changed files",
		ConditionPhrase: "all match patterns",
		ConditionValues: paths,
	}

	files, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	changedFiles := []string{}

	for _, f := range files {

		changedFiles = append(changedFiles, f.Filename)

		if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
			continue
		}
		predicateResult.Values = []string{f.Filename}
		predicateResult.Description = "A changed file does not match the required pattern"
		predicateResult.Satisfied = false
		return &predicateResult, nil
	}

	filesChanged := len(files) > 0

	desc := ""
	if !filesChanged {
		desc = "No files changed"
	}

	predicateResult.Values = changedFiles
	predicateResult.Description = desc
	predicateResult.Satisfied = filesChanged
	return &predicateResult, nil
}

func (pred *OnlyChangedFiles) Trigger() common.Trigger {
	return common.TriggerCommit
}

type NoChangedFiles struct {
	Paths       []common.Regexp `yaml:"paths,omitempty"`
	Globs       []common.Glob   `yaml:"globs,omitempty"`
	IgnorePaths []common.Regexp `yaml:"ignore,omitempty"`
	IgnoreGlobs []common.Glob   `yaml:"ignore_globs,omitempty"`
}

var _ Predicate = &NoChangedFiles{}

func (pred *NoChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	changedFiles := ChangedFiles{
		Paths:       pred.Paths,
		Globs:       pred.Globs,
		IgnorePaths: pred.IgnorePaths,
		IgnoreGlobs: pred.IgnoreGlobs,
	}

	changedFilesPredicateResult, err := changedFiles.Evaluate(ctx, prctx)
	if err != nil {
		return nil, err
	}

	predicateResult := common.PredicateResult{
		ValuePhrase:       "changed files",
		ConditionPhrase:   "match",
		ReverseSkipPhrase: true,
		Satisfied:         !changedFilesPredicateResult.Satisfied,
		Values:            changedFilesPredicateResult.Values,
		ConditionsMap:     changedFilesPredicateResult.ConditionsMap,
	}

	if predicateResult.Satisfied {
		predicateResult.Description = "No changed files match the specified patterns"
	} else {
		predicateResult.Description = changedFilesPredicateResult.Description
	}

	return &predicateResult, nil
}

func (pred *NoChangedFiles) Trigger() common.Trigger {
	return common.TriggerCommit
}

type FileAdded struct {
	Paths []common.Regexp `yaml:"paths,omitempty"`
	Globs []common.Glob   `yaml:"globs,omitempty"`
}

var _ Predicate = &FileAdded{}

func (pred *FileAdded) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)
	predicateResult := common.PredicateResult{
		Satisfied:       false,
		ValuePhrase:     "added files",
		Values:          []string{},
		ConditionPhrase: "match path patterns",
		ConditionValues: paths,
	}

	changedFiles, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	addedFiles := []string{}
	for _, f := range changedFiles {
		if f.Status == pull.FileAdded {
			addedFiles = append(addedFiles, f.Filename)

			if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
				predicateResult.Satisfied = true
				predicateResult.Values = []string{f.Filename}
				predicateResult.Description = f.Filename + " was added"
				return &predicateResult, nil
			}
		}
	}

	predicateResult.Values = addedFiles
	predicateResult.Description = "No added files match the specified patterns"
	return &predicateResult, nil
}

func (pred *FileAdded) Trigger() common.Trigger {
	return common.TriggerCommit
}

type FileNotAdded struct {
	Paths []common.Regexp `yaml:"paths,omitempty"`
	Globs []common.Glob   `yaml:"globs,omitempty"`
}

var _ Predicate = &FileNotAdded{}

func (pred *FileNotAdded) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)

	predicateResult := common.PredicateResult{
		Satisfied:         true,
		ValuePhrase:       "added files",
		Values:            []string{},
		ConditionPhrase:   "match path patterns",
		ConditionValues:   paths,
		ReverseSkipPhrase: true,
	}

	changedFiles, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	addedFiles := []string{}
	for _, f := range changedFiles {
		if f.Status == pull.FileAdded {
			addedFiles = append(addedFiles, f.Filename)

			if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
				predicateResult.Satisfied = false
				predicateResult.Values = []string{f.Filename}
				predicateResult.Description = f.Filename + " was added"
				return &predicateResult, nil
			}
		}
	}

	predicateResult.Values = addedFiles
	predicateResult.Description = "No added files match the specified patterns"
	return &predicateResult, nil
}

func (pred *FileNotAdded) Trigger() common.Trigger {
	return common.TriggerCommit
}

type FileDeleted struct {
	Paths []common.Regexp `yaml:"paths,omitempty"`
	Globs []common.Glob   `yaml:"globs,omitempty"`
}

var _ Predicate = &FileDeleted{}

func (pred *FileDeleted) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)

	predicateResult := common.PredicateResult{
		Satisfied:       false,
		ValuePhrase:     "deleted files",
		Values:          []string{},
		ConditionPhrase: "match path patterns",
		ConditionValues: paths,
	}

	changedFiles, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	deletedFiles := []string{}
	for _, f := range changedFiles {
		if f.Status == pull.FileDeleted {
			deletedFiles = append(deletedFiles, f.Filename)

			if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
				predicateResult.Satisfied = true
				predicateResult.Values = []string{f.Filename}
				predicateResult.Description = f.Filename + " was deleted"
				return &predicateResult, nil
			}
		}
	}

	predicateResult.Values = deletedFiles
	predicateResult.Description = "No deleted files match the specified patterns"
	return &predicateResult, nil
}

func (pred *FileDeleted) Trigger() common.Trigger {
	return common.TriggerCommit
}

type FileNotDeleted struct {
	Paths []common.Regexp `yaml:"paths,omitempty"`
	Globs []common.Glob   `yaml:"globs,omitempty"`
}

var _ Predicate = &FileNotDeleted{}

func (pred *FileNotDeleted) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	paths := getPathStrings(pred.Paths, pred.Globs)
	predicateResult := common.PredicateResult{
		Satisfied:         true,
		ValuePhrase:       "deleted files",
		Values:            []string{},
		ConditionPhrase:   "match path patterns",
		ConditionValues:   paths,
		ReverseSkipPhrase: true,
	}

	changedFiles, err := prctx.ChangedFiles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	deletedFiles := []string{}
	for _, f := range changedFiles {
		if f.Status == pull.FileDeleted {
			deletedFiles = append(deletedFiles, f.Filename)

			if anyMatches(pred.Paths, f.Filename) || anyGlobMatches(pred.Globs, f.Filename) {
				predicateResult.Satisfied = false
				predicateResult.Values = []string{f.Filename}
				predicateResult.Description = f.Filename + " was deleted"
				return &predicateResult, nil
			}
		}
	}

	predicateResult.Values = deletedFiles
	predicateResult.Description = "No deleted files match the specified patterns"
	return &predicateResult, nil
}

func (pred *FileNotDeleted) Trigger() common.Trigger {
	return common.TriggerCommit
}

type ModifiedLines struct {
	Additions ComparisonExpr          `yaml:"additions,omitempty"`
	Deletions ComparisonExpr          `yaml:"deletions,omitempty"`
	Total     ComparisonExpr          `yaml:"total,omitempty"`
	Files     ModifiedLinesFileFilter `yaml:"files,omitempty"`
}

type ModifiedLinesFileFilter struct {
	Include      []common.Regexp `yaml:"include,omitempty"`
	IncludeGlobs []common.Glob   `yaml:"include_globs,omitempty"`
	Exclude      []common.Regexp `yaml:"exclude,omitempty"`
	ExcludeGlobs []common.Glob   `yaml:"exclude_globs,omitempty"`
}

func (ff ModifiedLinesFileFilter) IsZero() bool {
	return len(ff.Include) == 0 && len(ff.IncludeGlobs) == 0 && len(ff.Exclude) == 0 && len(ff.ExcludeGlobs) == 0
}

func (ff ModifiedLinesFileFilter) MatchesFile(filename string) bool {
	if (len(ff.Exclude) > 0 || len(ff.ExcludeGlobs) > 0) && (anyMatches(ff.Exclude, filename) || anyGlobMatches(ff.ExcludeGlobs, filename)) {
		return false
	}
	if (len(ff.Include) > 0 || len(ff.IncludeGlobs) > 0) && !(anyMatches(ff.Include, filename) || anyGlobMatches(ff.IncludeGlobs, filename)) {
		return false
	}
	return true
}

type CompareOp uint8

const (
	OpNone CompareOp = iota
	OpLessThan
	OpGreaterThan
	OpEquals
)

type ComparisonExpr struct {
	Op    CompareOp
	Value int64
}

func (exp ComparisonExpr) IsEmpty() bool {
	return exp.Op == OpNone && exp.Value == 0
}

func (exp ComparisonExpr) Evaluate(n int64) bool {
	switch exp.Op {
	case OpLessThan:
		return n < exp.Value
	case OpGreaterThan:
		return n > exp.Value
	case OpEquals:
		return n == exp.Value
	}
	return false
}

func (exp ComparisonExpr) MarshalText() ([]byte, error) {
	if exp.Op == OpNone {
		return nil, nil
	}

	var op string
	switch exp.Op {
	case OpLessThan:
		op = "<"
	case OpGreaterThan:
		op = ">"
	case OpEquals:
		op = "="
	default:
		return nil, errors.Errorf("unknown operation: %d", exp.Op)
	}
	return []byte(fmt.Sprintf("%s %d", op, exp.Value)), nil
}

func (exp ComparisonExpr) String() string {
	res, err := exp.MarshalText()
	if err != nil {
		return fmt.Sprintf("?? (op:%d) %d", exp.Op, exp.Value)
	}
	return string(res[:])
}

func (exp *ComparisonExpr) UnmarshalText(text []byte) error {
	text = bytes.TrimSpace(text)
	if len(text) == 0 {
		*exp = ComparisonExpr{}
		return nil
	}

	i := 0
	var op CompareOp
	switch text[i] {
	case '<':
		op = OpLessThan
	case '>':
		op = OpGreaterThan
	case '=':
		op = OpEquals
	default:
		return errors.Errorf("invalid comparison operator: %c", text[i])
	}

	i++
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}

	v, err := strconv.ParseInt(string(text[i:]), 10, 64)
	if err != nil {
		return errors.Wrapf(err, "invalid comparison value")
	}

	*exp = ComparisonExpr{Op: op, Value: v}
	return nil
}

func (pred *ModifiedLines) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	files, err := prctx.ChangedFiles()

	predicateResult := common.PredicateResult{
		ValuePhrase:     "file modifications",
		ConditionPhrase: "meet",
		ConditionsMap:   make(map[string][]string),
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	var additions, deletions int64
	for _, f := range files {
		if !pred.Files.MatchesFile(f.Filename) {
			continue
		}
		additions += int64(f.Additions)
		deletions += int64(f.Deletions)
	}

	if len(pred.Files.Include) > 0 || len(pred.Files.IncludeGlobs) > 0 {
		predicateResult.ConditionsMap["in files matching"] = getPathStrings(pred.Files.Include, pred.Files.IncludeGlobs)
	}
	if len(pred.Files.Exclude) > 0 || len(pred.Files.ExcludeGlobs) > 0 {
		predicateResult.ConditionsMap["excluding files matching"] = getPathStrings(pred.Files.Exclude, pred.Files.ExcludeGlobs)
	}

	const conditionKey = "the modification conditions"

	if !pred.Additions.IsEmpty() {
		value := fmt.Sprintf("+%d", additions)
		cond := fmt.Sprintf("added lines %s", pred.Additions.String())

		if pred.Additions.Evaluate(additions) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	if !pred.Deletions.IsEmpty() {
		value := fmt.Sprintf("-%d", deletions)
		cond := fmt.Sprintf("deleted lines %s", pred.Deletions.String())

		if pred.Deletions.Evaluate(deletions) {
			predicateResult.Values = []string{value}
			predicateResult.ConditionsMap[conditionKey] = []string{cond}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}

		predicateResult.Values = append(predicateResult.Values, value)
		predicateResult.ConditionsMap[conditionKey] = append(predicateResult.ConditionsMap[conditionKey], cond)
	}

	if !pred.Total.IsEmpty() {
		value := fmt.Sprintf("total %d", additions+deletions)
		cond := fmt.Sprintf("total modifications %s", pred.Total.String())

		if pred.Total.Evaluate(additions + deletions) {
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

func (pred *ModifiedLines) Trigger() common.Trigger {
	return common.TriggerCommit
}

var _ Predicate = &ModifiedLines{}
