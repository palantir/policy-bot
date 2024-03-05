// Copyright 2020 Palantir Technologies, Inc.
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

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type CheckRun struct {
	Base
}

func (h *CheckRun) Handles() []string { return []string{"check_run"} }

func (h *CheckRun) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.CheckRunEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse check_run event payload")
	}

	if event.GetAction() != "completed" || event.GetCheckRun().GetConclusion() != "success" {
		return nil
	}

	repo := event.GetRepo()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetCheckRun().GetHeadSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	evaluationFailures := 0
	for _, pr := range event.GetCheckRun().PullRequests {
		// TODO(bkeyes): I'm assuming PRs in a check run are open at the time
		// of the event, but I can't find confirmation of that in the GitHub
		// docs. The PR object is a minimal version that is missing the "state"
		// field, so we can't check without loading the full object.
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
