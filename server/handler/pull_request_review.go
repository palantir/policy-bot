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

type PullRequestReview struct {
	Base
}

func (h *PullRequestReview) Handles() []string { return []string{"pull_request_review"} }

// Handle pull_request_review
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewevent
func (h *PullRequestReview) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.PullRequestReviewEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse pull request review event payload")
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

	ctx, _ = githubapp.PreparePRContext(ctx, installationID, event.GetRepo(), event.GetPullRequest().GetNumber())

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, event.GetRepo().GetOwner().GetLogin(), h.Installations, h.ClientCreator)
	return h.Evaluate(ctx, mbrCtx, client, v4client, event.GetPullRequest())
}
