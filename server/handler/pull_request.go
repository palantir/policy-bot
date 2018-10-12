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

type PullRequest struct {
	Base
}

func (h *PullRequest) Handles() []string { return []string{"pull_request"} }

// Handle pull_request
// https://developer.github.com/v3/activity/events/types/#requestevent
func (h *PullRequest) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.PullRequestEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse pull request event payload")
	}

	installationID := githubapp.GetInstallationIDFromEvent(&event)

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	ctx, _ = githubapp.PreparePRContext(ctx, installationID, event.GetRepo(), event.GetNumber())

	switch event.GetAction() {
	case "opened", "reopened", "synchronize", "edited":
		mbrCtx := NewCrossOrgMembershipContext(ctx, client, event.GetRepo().GetOwner().GetLogin(), h.Installations, h.ClientCreator)
		return h.Evaluate(ctx, mbrCtx, client, event.GetPullRequest())
	}

	return nil
}
