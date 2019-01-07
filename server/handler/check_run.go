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
)

type CheckRun struct {
	Base
}

func (h CheckRun) Handles() []string { return []string{"check_run"} }

// Handle check_run
// https://developer.github.com/v3/activity/events/types/#checkrunevent
func (h *CheckRun) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.CheckRunEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse check run event payload")
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
		for _, pullRequest := range event.GetCheckRun().PullRequests {
			ctx, _ = githubapp.PreparePRContext(ctx, installationID, event.GetRepo(), pullRequest.GetNumber())

			// HACK: This gets around a lack of context from the PR associated with a check run. As the API might
			// change later, this should be re-evaluated at a later date
			pullRequest.Base.Repo = event.GetRepo()
			mbrCtx := NewCrossOrgMembershipContext(ctx, client, event.GetRepo().GetOwner().GetLogin(), h.Installations, h.ClientCreator)
			return h.Evaluate(ctx, mbrCtx, client, v4client, pullRequest)
		}
	}

	return nil
}
