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

	"github.com/google/go-github/v81/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

type WorkflowRun struct {
	Base
}

func (h *WorkflowRun) Handles() []string { return []string{"workflow_run"} }

func (h *WorkflowRun) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) (err error) {
	// https://docs.github.com/en/actions/using-workflows/events-that-trigger-workflows#workflow_run
	// https://docs.github.com/en/webhooks/webhook-events-and-payloads?actionType=completed#workflow_run
	ctx, span := StartWebhookSpan(ctx, eventType, deliveryID)
	defer span.End()
	defer RecordError(span, &err)

	var event github.WorkflowRunEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse workflow_run event payload")
	}

	span.SetAttributes(attribute.String(AttrEventAction, event.GetAction()))

	if event.GetAction() != "completed" {
		span.SetAttributes(attribute.String(AttrPolicySkipReason, SkipReasonActionNotHandled))
		return nil
	}

	repo := event.GetRepo()
	repoID := repo.GetID()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetWorkflowRun().GetHeadSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	span.SetAttributes(RepoAttrs(ownerName, repoName)...)
	span.SetAttributes(
		attribute.String(AttrSHA, commitSHA),
		attribute.Int64(AttrInstallationID, installationID),
	)

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	evaluationFailures := 0
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

		prCtx, prSpan := StartChildSpan(ctx, "policy.evaluate_pr")
		prSpan.SetAttributes(
			attribute.Int(AttrPRNumber, pr.GetNumber()),
			attribute.String(AttrSHA, commitSHA),
		)
		if evalErr := h.Evaluate(prCtx, installationID, common.TriggerStatus, pull.Locator{
			Owner:  ownerName,
			Repo:   repoName,
			Number: pr.GetNumber(),
			Value:  pr,
		}); evalErr != nil {
			evaluationFailures++
			logger.Error().Err(evalErr).Msgf("Failed to evaluate pull request '%d' for SHA '%s'", pr.GetNumber(), commitSHA)
			RecordError(prSpan, &evalErr)
		}
		prSpan.End()
	}
	if evaluationFailures == 0 {
		return nil
	}

	return errors.Errorf("failed to evaluate %d pull requests", evaluationFailures)
}
