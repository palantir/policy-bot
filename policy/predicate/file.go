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
	"regexp"
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

type ComparisonExpr string

var (
	numCompRegexp = regexp.MustCompile(`^(<|>) ?(\d+)$`)
)

func (exp ComparisonExpr) IsEmpty() bool {
	return exp == ""
}

func (exp ComparisonExpr) Evaluate(n int64) (bool, error) {
	match := numCompRegexp.FindStringSubmatch(string(exp))
	if match == nil {
		return false, errors.Errorf("invalid comparison expression: %q", exp)
	}

	op := match[1]
	v, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return false, errors.Wrapf(err, "invalid commparison expression: %q", exp)
	}

	switch op {
	case "<":
		return n < v, nil
	case ">":
		return n > v, nil
	}
	return false, errors.Errorf("invalid comparison expression: %q", exp)
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
		if !expr.IsEmpty() {
			ok, err := expr.Evaluate(v)
			if err != nil {
				return false, "", err
			}
			if ok {
				return true, "", nil
			}
		}
	}
	return false, fmt.Sprintf("modification of (+%d, -%d) does not match any conditions", additions, deletions), nil
}

func (pred *ModifiedLines) Trigger() common.Trigger {
	return common.TriggerCommit
}

var _ Predicate = &ModifiedLines{}
