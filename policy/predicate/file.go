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
	"regexp"

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/pull"
)

type ChangedFiles struct {
	Paths []string `yaml:"paths"`
}

var _ Predicate = &ChangedFiles{}

func (pred *ChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	paths, err := pathsToRegexps(pred.Paths)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to parse paths")
	}

	files, err := prctx.ChangedFiles()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list changed files")
	}

	for _, f := range files {
		if anyMatches(paths, f.Filename) {
			return true, "", nil
		}
	}

	desc := "No changed files match the required patterns"
	return false, desc, nil
}

type OnlyChangedFiles struct {
	Paths []string `yaml:"paths"`
}

var _ Predicate = &OnlyChangedFiles{}

func (pred *OnlyChangedFiles) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	paths, err := pathsToRegexps(pred.Paths)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to parse paths")
	}

	files, err := prctx.ChangedFiles()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to list changed files")
	}

	for _, f := range files {
		if anyMatches(paths, f.Filename) {
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

func anyMatches(re []*regexp.Regexp, s string) bool {
	for _, r := range re {
		if r.MatchString(s) {
			return true
		}
	}
	return false
}

func pathsToRegexps(paths []string) ([]*regexp.Regexp, error) {
	var re []*regexp.Regexp
	for _, p := range paths {
		r, err := regexp.Compile(p)
		if err != nil {
			return re, err
		}
		re = append(re, r)
	}
	return re, nil
}
