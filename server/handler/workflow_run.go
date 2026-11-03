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
	"encoding/json"
	stderrors "errors"

	"github.com/google/go-github/v92/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type WorkflowRun struct {
	Base
}

func (h *WorkflowRun) Handles() []string { return []string{"workflow_run"} }

func (h *WorkflowRun) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	// https://docs.github.com/en/actions/using-workflows/events-that-trigger-workflows#workflow_run
	// https://docs.github.com/en/webhooks/webhook-events-and-payloads?actionType=completed#workflow_run
	var event github.WorkflowRunEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse workflow_run event payload")
	}

	if event.GetAction() != "completed" {
		return nil
	}

	repo := event.GetRepo()
	repoID := repo.GetID()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetWorkflowRun().GetHeadSHA()
	workflowPath := event.GetWorkflowRun().GetPath()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return errors.Wrap(err, "failed to create installation client")
	}

	var evaluationFailures []error
	for _, pr := range event.GetWorkflowRun().PullRequests {
		// The `workflow_run` event includes pull requests that contain the SHA
		// which is being checked. These can be pull requests _from_ our
		// repository _to_ another one, for example if it's been forked and
		// there's a PR to merge changes from our repo into the fork. We don't
		// want to try to evaluate the policy for such PRs as they're nothing to
		// do with us.
		prBaseRepo := pr.GetBase().GetRepo()
		if prBaseRepo.GetID() != repoID {
			logger.Debug().Msgf("Skipping pull request '%d' from different repository '%s'", pr.GetNumber(), prBaseRepo.GetURL())
			continue
		}

		// Only this workflow's own result reaches the policy through a workflow_run event,
		// so skip the evaluation when no rule names it. check_run and status events cover the rest.
		if h.ignoresWorkflow(ctx, client, ownerName, repoName, h.PullOpts.PolicyBranch(pr.GetBase().GetRef(), pr.GetHead().GetRef()), workflowPath) {
			logger.Debug().Msgf("Skipping pull request '%d': no rule references workflow '%s'", pr.GetNumber(), workflowPath)
			continue
		}

		if err := h.EvaluateCoalesced(ctx, installationID, common.TriggerStatus, pull.Locator{
			Owner:  ownerName,
			Repo:   repoName,
			Number: pr.GetNumber(),
			Value:  pr,
		}, event.GetWorkflowRun()); err != nil {
			evaluationFailures = append(evaluationFailures, err)
			logger.Error().Err(err).Msgf("Failed to evaluate pull request '%d' for SHA '%s'", pr.GetNumber(), commitSHA)
		}
	}
	if len(evaluationFailures) == 0 {
		return nil
	}

	// Joined rather than counted, so a retry can tell a transient failure from a permanent one
	return errors.Wrapf(stderrors.Join(evaluationFailures...), "failed to evaluate %d pull requests", len(evaluationFailures))
}

// ignoresWorkflow reports whether the policy on baseRef, or the default branch when empty, observes no result from workflowPath.
// A config that is missing, unreadable or invalid is never ignored, so the evaluation still runs and reports it.
func (h *WorkflowRun) ignoresWorkflow(ctx context.Context, client *github.Client, owner, repo, baseRef, workflowPath string) bool {
	if workflowPath == "" {
		return false
	}

	fetched := h.ConfigFetcher.ConfigForRepositoryBranch(ctx, client, owner, repo, baseRef)
	if fetched.Config == nil || fetched.LoadError != nil || fetched.ParseError != nil {
		return false
	}

	_, ok := fetched.Config.ReferencedWorkflows()[workflowPath]
	return !ok
}
