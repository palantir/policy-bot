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

func (pred *ChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	files, err := prctx.ChangedFiles()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list changed files")
	}

	for _, f := range files {
		if anyMatches(pred.IgnorePaths, f.Filename) {
			continue
		}

		if anyMatches(pred.Paths, f.Filename) {
			return true, f.Filename + " was changed", nil
		}
	}

	desc := "No changed files match the required patterns"
	return false, desc, nil
}

func (pred *ChangedFiles) Trigger() common.Trigger {
	return common.TriggerCommit
}

type OnlyChangedFiles struct {
	Paths []common.Regexp `yaml:"paths"`
}

var _ Predicate = &OnlyChangedFiles{}

func (pred *OnlyChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	files, err := prctx.ChangedFiles()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list changed files")
	}

	for _, f := range files {
		if anyMatches(pred.Paths, f.Filename) {
			continue
		}
		desc := "A changed file does not match the required pattern"
		return false, desc, nil
	}

	filesChanged := len(files) > 0

	desc := ""
	if !filesChanged {
		desc = "No files changed"
	}

	return filesChanged, desc, nil
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

func (pred *ModifiedLines) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	files, err := prctx.ChangedFiles()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list changed files")
	}

	var additions, deletions int64
	for _, f := range files {
		additions += int64(f.Additions)
		deletions += int64(f.Deletions)
	}

	for expr, v := range map[ComparisonExpr]int64{
		pred.Additions: additions,
		pred.Deletions: deletions,
		pred.Total:     additions + deletions,
	} {
		if !expr.IsEmpty() && expr.Evaluate(v) {
			return true, "", nil
		}
	}
	return false, fmt.Sprintf("modification of (+%d, -%d) does not match any conditions", additions, deletions), nil
}

func (pred *ModifiedLines) Trigger() common.Trigger {
	return common.TriggerCommit
}

var _ Predicate = &ModifiedLines{}
