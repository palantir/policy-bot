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

	"github.com/google/go-github/v93/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
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
	ctx, _ = h.PreparePRContext(ctx, installationID, event.GetPullRequest())

	t, ok := pullRequestTrigger(&event)
	if !ok {
		return nil
	}

	return h.Evaluate(ctx, installationID, t, pull.Locator{
		Owner:  event.GetRepo().GetOwner().GetLogin(),
		Repo:   event.GetRepo().GetName(),
		Number: event.GetPullRequest().GetNumber(),
		Value:  event.GetPullRequest(),
	})
}

// pullRequestTrigger returns the trigger for a pull_request event and false if
// the event action should not cause an evaluation.
func pullRequestTrigger(event *github.PullRequestEvent) (common.Trigger, bool) {
	switch event.GetAction() {
	case "opened", "reopened", "ready_for_review":
		return common.TriggerCommit | common.TriggerPullRequest, true
	case "synchronize":
		return common.TriggerCommit, true
	case "edited":
		// a base change alters the diff and policy but only produces an edited event
		if event.GetChanges().GetBase() != nil {
			return common.TriggerAll, true
		}
		return common.TriggerPullRequest, true
	case "labeled", "unlabeled":
		return common.TriggerLabel, true
	}
	return 0, false
}
