// Copyright 2026 Palantir Technologies, Inc.
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

package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var testCmdConfig struct {
	Policy   string
	Fixtures []string
}

var TestCmd = &cobra.Command{
	Use:   "test",
	Short: "Evaluates a policy against local scenario fixtures without calling GitHub.",
	Long: "Evaluates a local policy file against one or more scenario fixtures and checks that each\n" +
		"scenario produces the expected result. Scenarios describe a pull request (author, labels,\n" +
		"reviews, changed files, and so on) entirely offline, so policies can be unit tested in CI.\n" +
		"Remote policy references are not supported.",

	RunE: testCmd,
}

// validExpectations is the set of statuses a scenario may expect, matching
// common.EvaluationStatus.String().
var validExpectations = map[string]bool{
	"skipped":     true,
	"pending":     true,
	"approved":    true,
	"disapproved": true,
}

func testCmd(cmd *cobra.Command, args []string) error {
	if len(testCmdConfig.Fixtures) == 0 {
		return errors.New("at least one fixtures file is required (--fixtures)")
	}

	evaluator, err := loadPolicyEvaluator(testCmdConfig.Policy)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var total, failures int
	for _, path := range testCmdConfig.Fixtures {
		fixtures, err := loadFixtures(path)
		if err != nil {
			return err
		}
		for _, sc := range fixtures.Scenarios {
			total++
			if !validExpectations[sc.Expected] {
				return errors.Errorf("scenario %q has invalid expected value %q (want one of skipped, pending, approved, disapproved)", sc.Name, sc.Expected)
			}
			res := evaluator.Evaluate(context.Background(), sc.toContext())
			if res.Error != nil {
				failures++
				_, _ = fmt.Fprintf(out, "FAIL %s: evaluation error: %v\n", sc.Name, res.Error)
				continue
			}
			got := res.Status.String()
			if got != sc.Expected {
				failures++
				_, _ = fmt.Fprintf(out, "FAIL %s: expected %s, got %s (%s)\n", sc.Name, sc.Expected, got, res.StatusDescription)
				continue
			}
			_, _ = fmt.Fprintf(out, "PASS %s\n", sc.Name)
		}
	}

	_, _ = fmt.Fprintf(out, "\n%d scenario(s), %d passed, %d failed\n", total, total-failures, failures)
	if failures > 0 {
		return errors.Errorf("%d of %d scenario(s) failed", failures, total)
	}
	return nil
}

// loadPolicyEvaluator reads and parses a local policy file into an evaluator,
// mirroring the parsing done by the validate command. Remote policy references
// cannot be evaluated offline and are rejected.
func loadPolicyEvaluator(path string) (common.Evaluator, error) {
	policyData, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Errorf("policy file does not exist: %s", path)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read policy file: %s", path)
	}

	remoteRef, err := appconfig.YAMLRemoteRefParser("", policyData)
	if err == nil && remoteRef != nil {
		return nil, errors.New("remote policy references cannot be tested locally")
	}

	var policyConfig policy.Config
	if err := yaml.UnmarshalStrict(policyData, &policyConfig); err != nil {
		return nil, errors.Wrap(err, "failed to parse policy from yaml file")
	}
	evaluator, err := policy.ParsePolicy(&policyConfig, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse policy")
	}
	return evaluator, nil
}

type fixtureFile struct {
	Scenarios []scenario `yaml:"scenarios"`
}

// scenario describes a pull request state and the result a policy should
// produce for it. Field names are kept friendly; toContext maps them onto a
// pulltest.Context.
type scenario struct {
	Name     string `yaml:"name"`
	Expected string `yaml:"expected"`

	Author string `yaml:"author,omitempty"`
	Title  string `yaml:"title,omitempty"`
	Body   string `yaml:"body,omitempty"`
	State  string `yaml:"state,omitempty"`
	Draft  bool   `yaml:"draft,omitempty"`

	Base string `yaml:"base,omitempty"`
	Head string `yaml:"head,omitempty"`

	Labels       []string            `yaml:"labels,omitempty"`
	ChangedFiles []scenarioFile      `yaml:"changed_files,omitempty"`
	Comments     []scenarioComment   `yaml:"comments,omitempty"`
	Reviews      []scenarioReview    `yaml:"reviews,omitempty"`
	TeamMembers  map[string][]string `yaml:"team_memberships,omitempty"`
	OrgMembers   map[string][]string `yaml:"org_memberships,omitempty"`
	LatestStatus map[string]string   `yaml:"latest_statuses,omitempty"`
}

type scenarioFile struct {
	Name      string `yaml:"name"`
	Status    string `yaml:"status,omitempty"`
	Additions int    `yaml:"additions,omitempty"`
	Deletions int    `yaml:"deletions,omitempty"`
}

type scenarioComment struct {
	Author string `yaml:"author"`
	Body   string `yaml:"body"`
}

type scenarioReview struct {
	Author string `yaml:"author"`
	State  string `yaml:"state"`
	SHA    string `yaml:"sha,omitempty"`
	Body   string `yaml:"body,omitempty"`
}

var fileStatuses = map[string]pull.FileStatus{
	"":         pull.FileModified,
	"modified": pull.FileModified,
	"added":    pull.FileAdded,
	"deleted":  pull.FileDeleted,
}

func (s scenario) toContext() *pulltest.Context {
	state := s.State
	if state == "" {
		state = "open"
	}

	ctx := &pulltest.Context{
		AuthorValue:         s.Author,
		TitleValue:          s.Title,
		StateValue:          state,
		Draft:               s.Draft,
		BranchBaseName:      s.Base,
		BranchHeadName:      s.Head,
		LabelsValue:         s.Labels,
		TeamMemberships:     s.TeamMembers,
		OrgMemberships:      s.OrgMembers,
		LatestStatusesValue: s.LatestStatus,
	}

	if s.Body != "" {
		ctx.BodyValue = &pull.Body{Body: s.Body, Author: s.Author}
	}

	for _, f := range s.ChangedFiles {
		ctx.ChangedFilesValue = append(ctx.ChangedFilesValue, &pull.File{
			Filename:  f.Name,
			Status:    fileStatuses[f.Status],
			Additions: f.Additions,
			Deletions: f.Deletions,
		})
	}
	for _, c := range s.Comments {
		ctx.CommentsValue = append(ctx.CommentsValue, &pull.Comment{
			Author: c.Author,
			Body:   c.Body,
		})
	}
	for _, r := range s.Reviews {
		ctx.ReviewsValue = append(ctx.ReviewsValue, &pull.Review{
			Author: r.Author,
			State:  pull.ReviewState(r.State),
			SHA:    r.SHA,
			Body:   r.Body,
		})
	}

	return ctx
}

func loadFixtures(path string) (*fixtureFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Errorf("fixtures file does not exist: %s", path)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read fixtures file: %s", path)
	}

	var fixtures fixtureFile
	if err := yaml.UnmarshalStrict(data, &fixtures); err != nil {
		return nil, errors.Wrapf(err, "failed to parse fixtures file: %s", path)
	}
	if len(fixtures.Scenarios) == 0 {
		return nil, errors.Errorf("fixtures file contains no scenarios: %s", path)
	}
	return &fixtures, nil
}

func init() {
	RootCmd.AddCommand(TestCmd)

	TestCmd.Flags().StringVarP(&testCmdConfig.Policy, "policy", "p", ".policy.yml", "path to the policy file to test")
	TestCmd.Flags().StringSliceVarP(&testCmdConfig.Fixtures, "fixtures", "f", nil, "path(s) to scenario fixture file(s)")
}
