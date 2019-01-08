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

	"github.com/google/go-github/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type CheckSuite struct {
	Base
}

func (h CheckSuite) Handles() []string { return []string{"check_suite"} }

// Handle check_run
// https://developer.github.com/v3/activity/events/types/#checksuiteevent
func (h *CheckSuite) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.CheckSuiteEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse check suite event payload")
	}

	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	v4client, err := h.NewInstallationV4Client(installationID)
	if err != nil {
		return err
	}

	switch event.GetAction() {
	case "rerequested":
		owner := event.GetRepo().GetOwner().GetLogin()
		repo := event.GetRepo().GetName()

		for _, checkPullRequest := range event.GetCheckSuite().PullRequests {
			pullRequestId := checkPullRequest.GetNumber()

			ctx, _ = githubapp.PreparePRContext(ctx, installationID, event.GetRepo(), pullRequestId)
			logger := zerolog.Ctx(ctx)

			// Load up the PR dependant affected by a check suite update
			pullRequest, _, err := client.PullRequests.Get(ctx, owner, repo, pullRequestId)
			if err != nil {
				logger.Error().Err(err).Msgf("unable to load pull request %s in %s/%s", pullRequestId, owner, repo)
				continue
			}

			mbrCtx := NewCrossOrgMembershipContext(ctx, client, owner, h.Installations, h.ClientCreator)
			err = h.Evaluate(ctx, mbrCtx, client, v4client, pullRequest)
			if err != nil {
				logger.Error().Err(err).Msgf("unable to process checks for pull request %s in %s/%s", pullRequestId, owner, repo)
			}
		}
	}

	return nil
}
