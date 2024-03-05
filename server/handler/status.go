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
	"fmt"
	"strings"

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type Status struct {
	Base
}

func (h *Status) Handles() []string { return []string{"status"} }

// Handle status
// https://developer.github.com/v3/activity/events/types/#statusevent
func (h *Status) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.StatusEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse status event payload")
	}

	if strings.HasPrefix(event.GetContext(), h.PullOpts.StatusCheckContext) {
		return h.processOwn(ctx, event)
	}

	if event.GetState() == "success" {
		return h.processOthers(ctx, event)
	}

	return nil
}

func (h *Status) processOwn(ctx context.Context, event github.StatusEvent) error {
	repo := event.GetRepo()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetCommit().GetSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)
	sender := event.GetSender()

	if sender.GetLogin() == h.AppName+"[bot]" {
		return nil
	}

	logger.Warn().
		Str(LogKeyAudit, event.GetName()).
		Str(LogKeyGitHubSHA, commitSHA).
		Msgf(
			"Entity '%s' overwrote status check '%s' to state='%s' description='%s' targetURL='%s'",
			sender.GetLogin(),
			event.GetContext(),
			event.GetState(),
			event.GetDescription(),
			event.GetTargetURL(),
		)

	// must be less than 140 characters to satisfy GitHub API
	desc := fmt.Sprintf("'%s' overwrote status to '%s'", sender.GetLogin(), event.GetState())

	// unlike in other code, use a single context here because we want to
	// replace a forged context with a failure, not post a general status
	// if multiple contexts are forged, we will handle multiple events
	status := &github.RepoStatus{
		Context:     event.Context,
		State:       github.String("failure"),
		Description: &desc,
	}

	_, _, err = client.Repositories.CreateStatus(ctx, ownerName, repoName, commitSHA, status)
	return err
}

func (h *Status) processOthers(ctx context.Context, event github.StatusEvent) error {
	repo := event.GetRepo()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	commitSHA := event.GetCommit().GetSHA()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	// In practice, there should be well under 100 PRs for a given commit. In exceptional cases, if there are
	// more than 100 PRs, only process the most recent 100.
	prs, _, err := client.PullRequests.ListPullRequestsWithCommit(
		ctx,
		ownerName,
		repoName,
		commitSHA,
		&github.ListOptions{
			PerPage: 100,
		})
	if err != nil {
		return errors.Wrapf(err, "failed to list pull requests for SHA %s", commitSHA)
	}
	logger.Debug().Msgf("Context event is for '%s', found %d PRs", event.GetContext(), len(prs))

	evaluationFailures := 0
	for _, pr := range prs {
		if pr.GetState() == "open" {
			err = h.Evaluate(ctx, installationID, common.TriggerStatus, pull.Locator{
				Owner:  ownerName,
				Repo:   repoName,
				Number: pr.GetNumber(),
				Value:  pr,
			})
			if err != nil {
				evaluationFailures++
				logger.Error().Err(err).Msgf("Failed to evaluate pull request '%d' for SHA '%s'", pr.GetNumber(),
					commitSHA)

			}
		}
	}
	if evaluationFailures == 0 {
		return nil
	}
	return errors.Errorf("failed to evaluate %d pull requests", evaluationFailures)
}
