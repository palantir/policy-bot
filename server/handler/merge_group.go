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

	"github.com/google/go-github/v50/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type MergeGroup struct {
	Base
}

func (h *MergeGroup) Handles() []string { return []string{"merge_group"} }

// Handle merge_group
// https://docs.github.com/webhooks-and-events/webhooks/webhook-events-and-payloads#merge_group
func (h *MergeGroup) Handle(ctx context.Context, eventType, devlieryID string, payload []byte) error {
	var event github.MergeGroupEvent
	repository := event.GetRepo().GetName()
	owner := event.GetOrg().GetName()
	mergeGroup := event.GetMergeGroup()
	baseBranch := mergeGroup.GetBaseRef()
	headSHA := mergeGroup.GetHeadSHA()

	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse merge group event payload")
	}

	if event.GetAction() != "checks_requested" {
		return nil
	}

	logger := zerolog.Ctx(ctx)
	installationID := githubapp.GetInstallationIDFromEvent(&event)
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	c, err := h.ConfigFetcher.Loader.LoadConfig(ctx, client, owner, repository, baseBranch)
	fc := FetchedConfig{
		Source: c.Source,
		Path:   c.Path,
	}

	switch {
	case err != nil:
		fc.LoadError = err
		msg := fmt.Sprintf("No policy defined on base branch: %s for repository: %s", baseBranch, repository)
		logger.Warn().Err(errors.WithStack(err)).Msg(msg)
		return nil
	case c.IsUndefined():
		return nil
	}

	contextWithBranch := fmt.Sprintf("%s: %s", h.PullOpts.StatusCheckContext, baseBranch)
	state := "success"
	message := fmt.Sprintf("%s previously approved original pull request.", h.AppName)
	status := &github.RepoStatus{
		Context:     &contextWithBranch,
		State:       &state,
		Description: &message,
	}

	if err := PostStatus(ctx, client, owner, repository, headSHA, status); err != nil {
		logger.Err(errors.WithStack(err)).Msg("Failed to post repo status")
	}

	return nil
}
