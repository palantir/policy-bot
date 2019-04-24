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

	"github.com/google/go-github/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
)

type Status struct {
	Base
}

func (h *Status) Handles() []string { return []string{"status"} }

// Handle pull_request
// https://developer.github.com/v3/activity/events/types/#statusevent
func (h *Status) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.StatusEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse status event payload")
	}

	repo := event.GetRepo()
	ownerName := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	ctx, logger := githubapp.PrepareRepoContext(ctx, installationID, repo)

	// ignore contexts that are not ours
	if !strings.HasPrefix(event.GetContext(), h.PullOpts.StatusCheckContext) {
		logger.Debug().Msgf("Ignoring context event for '%s'", event.GetContext())
		return nil
	}

	sender := event.GetSender()
	commitSHA := event.GetCommit().GetSHA()

	if sender.GetLogin() != h.PullOpts.AppName+"[bot]" {
		logger.Warn().
			Str(LogKeyAudit, eventType).
			Str(GitHubSHAKey, commitSHA).
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

		_, _, err := client.Repositories.CreateStatus(ctx, ownerName, repoName, commitSHA, status)
		return err
	}

	return nil
}
