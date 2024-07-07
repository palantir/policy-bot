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

	"github.com/google/go-github/v63/github"
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
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetWorkflowRun().GetHeadSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	evaluationFailures := 0
	for _, pr := range event.GetWorkflowRun().PullRequests {
		if err := h.Evaluate(ctx, installationID, common.TriggerStatus, pull.Locator{
			Owner:  ownerName,
			Repo:   repoName,
			Number: pr.GetNumber(),
			Value:  pr,
		}); err != nil {
			evaluationFailures++
			logger.Error().Err(err).Msgf("Failed to evaluate pull request '%d' for SHA '%s'", pr.GetNumber(), commitSHA)
		}
	}
	if evaluationFailures == 0 {
		return nil
	}

	return errors.Errorf("failed to evaluate %d pull requests", evaluationFailures)
}
