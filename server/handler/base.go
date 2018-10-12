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

package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

const (
	DefaultPolicyPath         = ".policy.yml"
	DefaultStatusCheckContext = "policy-bot"
	DefaultAppName            = "policy-bot"
)

type Base struct {
	githubapp.ClientCreator

	Installations githubapp.InstallationsService
	PullOpts      *PullEvaluationOptions
	ConfigFetcher *ConfigFetcher
	BaseConfig    *baseapp.HTTPConfig
}

type PullEvaluationOptions struct {
	PolicyPath         string `yaml:"policy_path"`
	StatusCheckContext string `yaml:"status_check_context"`
	AppName            string `yaml:"app_name"`
}

func (p *PullEvaluationOptions) FillDefaults() {
	if p.PolicyPath == "" {
		p.PolicyPath = DefaultPolicyPath
	}

	if p.StatusCheckContext == "" {
		p.StatusCheckContext = DefaultStatusCheckContext
	}

	if p.AppName == "" {
		p.AppName = DefaultAppName
	}
}

func (b *Base) MakeStatus(state, message string, pr *github.PullRequest) *github.RepoStatus {
	var details string
	if pr != nil {
		publicURL := strings.TrimSuffix(b.BaseConfig.PublicURL, "/")
		details = fmt.Sprintf("%s/details/%s/%d", publicURL, pr.GetBase().GetRepo().GetFullName(), pr.GetNumber())
	}

	return &github.RepoStatus{
		Context:     &b.PullOpts.StatusCheckContext,
		State:       &state,
		Description: &message,
		TargetURL:   &details,
	}
}

func (b *Base) Evaluate(ctx context.Context, mbrCtx pull.MembershipContext, client *github.Client, pr *github.PullRequest) error {
	fetchedConfig, err := b.ConfigFetcher.ConfigForPR(ctx, client, pr)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("failed to fetch policy: %s", fetchedConfig))
	}
	return b.EvaluateFetchedConfig(ctx, mbrCtx, client, pr, fetchedConfig)
}

func (b *Base) EvaluateFetchedConfig(ctx context.Context, mbrCtx pull.MembershipContext, client *github.Client, pr *github.PullRequest, fetchedConfig FetchedConfig) error {
	logger := zerolog.Ctx(ctx)

	if fetchedConfig.Missing() {
		logger.Debug().Msgf("policy does not exist: %s", fetchedConfig)
		return nil
	}

	srcSHA := pr.GetHead().GetSHA()

	if fetchedConfig.Invalid() {
		logger.Warn().Err(fetchedConfig.Error).Msgf("invalid policy: %s", fetchedConfig)

		s := b.MakeStatus("error", fetchedConfig.Description(), pr)
		_, _, err := client.Repositories.CreateStatus(ctx, fetchedConfig.Owner, fetchedConfig.Repo, srcSHA, s)
		return err
	}

	evaluator, err := policy.ParsePolicy(fetchedConfig.Config)
	if err != nil {
		statusMessage := fmt.Sprintf("Invalid policy defined by %s", fetchedConfig)
		logger.Debug().Err(err).Msg(statusMessage)

		s := b.MakeStatus("error", statusMessage, pr)
		_, _, err := client.Repositories.CreateStatus(ctx, fetchedConfig.Owner, fetchedConfig.Repo, srcSHA, s)
		return err
	}

	prctx := pull.NewGitHubContext(ctx, mbrCtx, client, pr)
	result := evaluator.Evaluate(ctx, prctx)

	if result.Error != nil {
		statusMessage := fmt.Sprintf("Error evaluating policy defined by %s", fetchedConfig)
		logger.Warn().Err(result.Error).Msg(statusMessage)

		s := b.MakeStatus("error", statusMessage, pr)
		_, _, err := client.Repositories.CreateStatus(ctx, fetchedConfig.Owner, fetchedConfig.Repo, srcSHA, s)
		return err
	}

	statusDescription := result.Description
	var statusState string
	switch result.Status {
	case common.StatusApproved:
		statusState = "success"
	case common.StatusDisapproved:
		statusState = "failure"
	case common.StatusPending:
		statusState = "pending"
	case common.StatusSkipped:
		statusState = "error"
		statusDescription = "All rules were skipped. At least one rule must match."
	default:
		return errors.Errorf("evaluation resulted in unexpected state: %s", result.Status)
	}

	s := b.MakeStatus(statusState, statusDescription, pr)
	logger.Info().Msgf("Setting status context=%v state=%v description=%v target_url=%v", s.Context, s.State, s.Description, "")
	_, _, err = client.Repositories.CreateStatus(ctx, fetchedConfig.Owner, fetchedConfig.Repo, srcSHA, s)
	return err
}
