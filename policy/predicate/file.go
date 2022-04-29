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

type ChangedFiles struct {
	Paths       []common.Regexp `yaml:"paths"`
	IgnorePaths []common.Regexp `yaml:"ignore"`
}

var _ Predicate = &ChangedFiles{}

func (pred *ChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	var paths, ignorePaths []string

	for _, path := range pred.Paths {
		paths = append(paths, path.String())
	}

	for _, ignorePath := range pred.IgnorePaths {
		ignorePaths = append(ignorePaths, ignorePath.String())
	}

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

		if anyMatches(pred.IgnorePaths, f.Filename) {
			continue
		}

		if anyMatches(pred.Paths, f.Filename) {
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
	Paths []common.Regexp `yaml:"paths"`
}

var _ Predicate = &OnlyChangedFiles{}

func (pred *OnlyChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	var paths []string

	for _, path := range pred.Paths {
		paths = append(paths, path.String())
	}

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

		if anyMatches(pred.Paths, f.Filename) {
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

type ModifiedLines struct {
	Additions ComparisonExpr `yaml:"additions"`
	Deletions ComparisonExpr `yaml:"deletions"`
	Total     ComparisonExpr `yaml:"total"`
}

type CompareOp uint8

const (
	OpNone CompareOp = iota
	OpLessThan
	OpGreaterThan
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
		ConditionPhrase: "meet the modification conditions",
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to list changed files")
	}

	var additions, deletions int64
	for _, f := range files {
		additions += int64(f.Additions)
		deletions += int64(f.Deletions)
	}

	if !pred.Additions.IsEmpty() {
		predicateResult.Values = []string{fmt.Sprintf("+%d", additions)}
		predicateResult.ConditionValues = []string{fmt.Sprintf("added lines %s", pred.Additions.String())}
		if pred.Additions.Evaluate(additions) {
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}
	if !pred.Deletions.IsEmpty() {
		if pred.Deletions.Evaluate(deletions) {
			predicateResult.Values = []string{fmt.Sprintf("-%d", deletions)}
			predicateResult.ConditionValues = []string{fmt.Sprintf("deleted lines %s", pred.Deletions.String())}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
		predicateResult.Values = append(predicateResult.Values, fmt.Sprintf("-%d", deletions))
		predicateResult.ConditionValues = append(predicateResult.ConditionValues, fmt.Sprintf("deleted lines %s", pred.Deletions.String()))
	}
	if !pred.Total.IsEmpty() {
		if pred.Total.Evaluate(additions + deletions) {
			predicateResult.Values = []string{fmt.Sprintf("total %d", additions+deletions)}
			predicateResult.ConditionValues = []string{fmt.Sprintf("total modifications %s", pred.Total.String())}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
		predicateResult.Values = append(predicateResult.Values, fmt.Sprintf("total %d", additions+deletions))
		predicateResult.ConditionValues = append(predicateResult.ConditionValues, fmt.Sprintf("total modifications %s", pred.Total.String()))
	}
	predicateResult.Satisfied = false
	return &predicateResult, nil
}

func (pred *ModifiedLines) Trigger() common.Trigger {
	return common.TriggerCommit
}

var _ Predicate = &ModifiedLines{}
