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
	stderrors "errors"

	"github.com/google/go-github/v92/github"
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
	repoID := repo.GetID()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetCheckRun().GetHeadSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return errors.Wrap(err, "failed to create installation client")
	}

	checkName := event.GetCheckRun().GetName()
	logger.Debug().Msgf("Check run event is for '%s', found %d PRs", checkName, len(event.GetCheckRun().PullRequests))

	var evaluationFailures []error
	for _, pr := range event.GetCheckRun().PullRequests {
		// TODO(bkeyes): I'm assuming PRs in a check run are open at the time
		// of the event, but I can't find confirmation of that in the GitHub
		// docs. The PR object is a minimal version that is missing the "state"
		// field, so we can't check without loading the full object.

		// The `check_run` event includes pull requests that contain the SHA
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

		// A policy reads checks from the head commit only, so a check on a commit lower in a stack cannot change the result
		if pr.GetHead().GetSHA() != commitSHA {
			logger.Debug().Msgf("Skipping pull request '%d': check run is not on its head commit", pr.GetNumber())
			continue
		}

		// Every GitHub Actions job is a check run, and workflow_run events already cover the workflows the policy reads.
		// So only evaluate when some rule names this check.
		if h.ignoresStatus(ctx, client, ownerName, repoName, h.PullOpts.PolicyBranch(pr.GetBase().GetRef(), pr.GetHead().GetRef()), checkName) {
			logger.Debug().Msgf("Skipping pull request '%d': no rule references check '%s'", pr.GetNumber(), checkName)
			continue
		}

		if err := h.EvaluateCoalesced(ctx, installationID, common.TriggerStatus, pull.Locator{
			Owner:  ownerName,
			Repo:   repoName,
			Number: pr.GetNumber(),
			Value:  pr,
		}); err != nil {
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
