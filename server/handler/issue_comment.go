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

	"github.com/google/go-github/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/pull"
)

type IssueComment struct {
	Base
}

func (h *IssueComment) Handles() []string { return []string{"issue_comment"} }

// Handle issue_comment
// See https://developer.github.com/v3/activity/events/types/#issuecommentevent
func (h *IssueComment) Handle(ctx context.Context, eventType, deliveryID string, payload []byte) error {
	var event github.IssueCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.Wrap(err, "failed to parse issue comment event payload")
	}

	repo := event.GetRepo()
	owner := repo.GetOwner().GetLogin()
	number := event.GetIssue().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	if !event.GetIssue().IsPullRequest() {
		zerolog.Ctx(ctx).Debug().Msg("Issue comment event is not for a pull request")
		return nil
	}

	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	v4client, err := h.NewInstallationV4Client(installationID)
	if err != nil {
		return err
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo.GetName(), number)
	if err != nil {
		return errors.Wrapf(err, "failed to get pull request %s/%s#%d", owner, repo.GetName(), number)
	}

	ctx, logger := h.PreparePRContext(ctx, installationID, pr)

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, owner, h.Installations, h.ClientCreator)
	prctx, err := pull.NewGitHubContext(ctx, mbrCtx, client, v4client, pull.Locator{
		Owner:  owner,
		Repo:   repo.GetName(),
		Number: number,
		Value:  pr,
	})
	if err != nil {
		return err
	}

	fetchedConfig, err := h.ConfigFetcher.ConfigForPR(ctx, prctx, client)
	if err != nil {
		return errors.Wrap(err, "failed to fetch configuration")
	}

	if fetchedConfig.Valid() {
		tampered, err := h.detectAndLogTampering(ctx, prctx, client, event, fetchedConfig.Config)
		if err != nil {
			return errors.Wrap(err, "failed to detect tampering")
		} else if tampered {
			return nil
		}
	} else if !fetchedConfig.Missing() {
		logger.Warn().Str(LogKeyAudit, "issue_comment").Msg("Skipped tampering check because the policy is not valid")
	}

	return h.EvaluateFetchedConfig(ctx, prctx, client, fetchedConfig)
}

func (h *IssueComment) detectAndLogTampering(ctx context.Context, prctx pull.Context, client *github.Client, event github.IssueCommentEvent, config *policy.Config) (bool, error) {
	logger := zerolog.Ctx(ctx)

	var originalBody string
	switch event.GetAction() {
	case "edited":
		originalBody = *event.GetChanges().Body.From

	case "deleted":
		originalBody = event.GetComment().GetBody()

	default:
		return false, nil
	}

	eventAuthor := event.GetSender().GetLogin()
	commentAuthor := event.GetComment().GetUser().GetLogin()
	if eventAuthor == commentAuthor {
		return false, nil
	}

	if h.affectsApproval(originalBody, config.ApprovalRules) {
		msg := fmt.Sprintf("Entity %s edited approval comment by %s", eventAuthor, commentAuthor)
		logger.Warn().Str(LogKeyAudit, "issue_comment").Msg(msg)

		err := h.PostStatus(ctx, prctx, client, "failure", msg)
		return true, err
	}

	logger.Warn().Str(LogKeyAudit, "issue_comment").Msgf("The comment_editor=%s is not the author=%s", eventAuthor, commentAuthor)
	return true, nil
}

func (h *IssueComment) affectsApproval(actualComment string, rules []*approval.Rule) bool {
	for _, rule := range rules {
		if rule.Options.GetMethods().CommentMatches(actualComment) {
			return true
		}
	}

	return false
}
