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
	"time"

	"github.com/google/go-github/github"
	"github.com/palantir/go-baseapp/baseapp"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/shurcooL/githubv4"

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
	AppName    string `yaml:"app_name"`
	PolicyPath string `yaml:"policy_path"`

	// StatusCheckContext will be used to create the status context. It will be used in the following
	// pattern: <StatusCheckContext>: <Base Branch Name>
	StatusCheckContext string `yaml:"status_check_context"`

	// PostInsecureStatusChecks enables the sending of a second status using just StatusCheckContext as the context,
	// no templating. This is turned off by default. This is to support legacy workflows that depend on the original
	// context behaviour, and will be removed in 2.0
	PostInsecureStatusChecks bool `yaml:"post_insecure_status_checks"`

	// DisableGitHubStatusChecks disable the sending of a status update to GitHub. This is enable by default as
	// traditionally status checks were the only way to add status to a Pull Request. This can be replaced by the new
	// GitHub Checks feature. See EnableGitHubCheckRuns
	DisableGitHubStatusChecks bool `yaml:"disable_github_status_updates"`

	// EnableGitHubCheckRuns enables posting of the results of the policy evaluations as GitHub checks. This means that
	// that full results are visible on the checks tab of a GitHub pull request. By default this is turned off as the
	// `checks:write` permission is required for the GitHub application
	EnableGitHubCheckRuns bool `yaml:"enable_github_check_runs"`
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

func (b *Base) PostStatus(ctx context.Context, client *github.Client, pr *github.PullRequest, state, message string) error {
	owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repo := pr.GetBase().GetRepo().GetName()
	sha := pr.GetHead().GetSHA()

	publicURL := strings.TrimSuffix(b.BaseConfig.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, pr.GetNumber())

	contextWithBranch := fmt.Sprintf("%s: %s", b.PullOpts.StatusCheckContext, pr.GetBase().GetRef())
	status := &github.RepoStatus{
		Context:     &contextWithBranch,
		State:       &state,
		Description: &message,
		TargetURL:   &detailsURL,
	}

	if err := b.postGitHubRepoStatus(ctx, client, owner, repo, sha, status); err != nil {
		return err
	}

	if b.PullOpts.PostInsecureStatusChecks {
		status.Context = &b.PullOpts.StatusCheckContext
		if err := b.postGitHubRepoStatus(ctx, client, owner, repo, sha, status); err != nil {
			return err
		}
	}

	return nil
}

func (b *Base) postGitHubRepoStatus(ctx context.Context, client *github.Client, owner, repo, ref string, status *github.RepoStatus) error {
	logger := zerolog.Ctx(ctx)
	logger.Info().Msgf("Setting status context=%s state=%s description=%s target_url=%s", status.GetContext(), status.GetState(), status.GetDescription(), status.GetTargetURL())
	_, _, err := client.Repositories.CreateStatus(ctx, owner, repo, ref, status)
	return err
}

func (b *Base) PostCheckResults(ctx context.Context, client *github.Client, pr *github.PullRequest, evaluationResult common.Result) error {
	sha := pr.GetHead().GetSHA()
	owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repo := pr.GetBase().GetRepo().GetName()

	for _, result := range evaluationResult.Children {
		if err := b.createGitHubRepoCheck(ctx, client, pr, owner, repo, sha, *result); err != nil {
			return err
		}
	}

	return nil
}

func (b *Base) createGitHubRepoCheck(ctx context.Context, client *github.Client, pr *github.PullRequest, owner string, repo string, sha string, evaluationResult common.Result) error {
	publicURL := strings.TrimSuffix(b.BaseConfig.PublicURL, "/")
	detailsURL := fmt.Sprintf("%s/details/%s/%s/%d", publicURL, owner, repo, pr.GetNumber())
	defaultStatus := "completed"
	time := github.Timestamp{
		Time: time.Now(),
	}

	conclusion := "neutral"
	switch evaluationResult.Status.String() {
	case "approved":
		conclusion = "success"
	case "skipped":
		conclusion = "neutral"
	case "disapproved":
		conclusion = "failure"
	case "unknown":
		conclusion = "failure"
	case "pending":
		conclusion = "failure"
	}

	var builder strings.Builder
	if len(evaluationResult.Children) == 0 {
		builder.WriteString(evaluationResult.Description)
	}

	childSummaries := b.generateCheckRunSummary(evaluationResult.Children, 0)
	builder.WriteString(childSummaries)
	summary := builder.String()

	status := &github.CreateCheckRunOptions{
		Name:       strings.Title(evaluationResult.Name + "s"),
		DetailsURL: &detailsURL,
		Status:     &defaultStatus,
		Output: &github.CheckRunOutput{
			Title:   &evaluationResult.Description,
			Summary: &summary,
		},
		StartedAt:   &time,
		CompletedAt: &time,
		Conclusion:  &conclusion,
		HeadSHA:     sha,
	}

	_, _, err := client.Checks.CreateCheckRun(ctx, owner, repo, *status)
	return err
}

func (b *Base) generateCheckRunSummary(results []*common.Result, level int) string {
	var summaryBuilder strings.Builder
	headerValue := strings.Repeat("#", 3+level)
	indentValue := strings.Repeat(" ", 2*level)

	for _, childResult := range results {
		summaryBuilder.WriteString("\n")
		summaryBuilder.WriteString(fmt.Sprintf("%s- %s %s (%s)", indentValue, headerValue, strings.Title(childResult.Name), childResult.Status.String()))
		summaryBuilder.WriteString("\n")
		summaryBuilder.WriteString(fmt.Sprintf("  %s%s", indentValue, childResult.Description))
		childSummaries := b.generateCheckRunSummary(childResult.Children, level+1)
		summaryBuilder.WriteString(childSummaries)
	}

	return summaryBuilder.String()
}

func (b *Base) Evaluate(ctx context.Context, mbrCtx pull.MembershipContext, client *github.Client, v4client *githubv4.Client, pr *github.PullRequest) error {
	fetchedConfig, err := b.ConfigFetcher.ConfigForPR(ctx, client, pr)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("failed to fetch policy: %s", fetchedConfig))
	}
	return b.EvaluateFetchedConfig(ctx, mbrCtx, client, v4client, pr, fetchedConfig)
}

func (b *Base) EvaluateFetchedConfig(ctx context.Context, mbrCtx pull.MembershipContext, client *github.Client, v4client *githubv4.Client, pr *github.PullRequest, fetchedConfig FetchedConfig) error {
	logger := zerolog.Ctx(ctx)

	if fetchedConfig.Missing() {
		logger.Debug().Msgf("policy does not exist: %s", fetchedConfig)
		return nil
	}

	if fetchedConfig.Invalid() {
		logger.Warn().Err(fetchedConfig.Error).Msgf("invalid policy: %s", fetchedConfig)
		err := b.PostStatus(ctx, client, pr, "error", fetchedConfig.Description())
		return err
	}

	evaluator, err := policy.ParsePolicy(fetchedConfig.Config)
	if err != nil {
		statusMessage := fmt.Sprintf("Invalid policy defined by %s", fetchedConfig)
		logger.Debug().Err(err).Msg(statusMessage)
		err := b.PostStatus(ctx, client, pr, "error", statusMessage)
		return err
	}

	prctx := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, pr)
	result := evaluator.Evaluate(ctx, prctx)

	if result.Error != nil {
		statusMessage := fmt.Sprintf("Error evaluating policy defined by %s", fetchedConfig)
		logger.Warn().Err(result.Error).Msg(statusMessage)
		err := b.PostStatus(ctx, client, pr, "error", statusMessage)
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

	if !b.PullOpts.DisableGitHubStatusChecks {
		err = b.PostStatus(ctx, client, pr, statusState, statusDescription)
		if err != nil {
			return err
		}
	}

	if b.PullOpts.EnableGitHubCheckRuns {
		err = b.PostCheckResults(ctx, client, pr, result)
	}
	return err
}
