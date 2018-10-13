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

	if !event.GetIssue().IsPullRequest() {
		zerolog.Ctx(ctx).Debug().Msg("Issue comment event is not for a pull request")
		return nil
	}

	repo := event.GetRepo()
	number := event.GetIssue().GetNumber()
	installationID := githubapp.GetInstallationIDFromEvent(&event)

	ctx, logger := githubapp.PreparePRContext(ctx, installationID, event.GetRepo(), number)
	client, err := h.NewInstallationClient(installationID)
	if err != nil {
		return err
	}

	pr, _, err := client.PullRequests.Get(ctx, repo.GetOwner().GetLogin(), repo.GetName(), number)
	if err != nil {
		return errors.Wrapf(err, "failed to get pull request %s/%s#%d", repo.GetOwner().GetLogin(), repo.GetName(), number)
	}

	fetchedConfig, err := h.ConfigFetcher.ConfigForPR(ctx, client, pr)
	if err != nil {
		return errors.Wrap(err, "failed to fetch configuration")
	}

	if fetchedConfig.Valid() {
		tampered, err := h.detectAndLogTampering(ctx, client, event, pr, fetchedConfig.Config)
		if err != nil {
			return errors.Wrap(err, "failed to detect tampering")
		} else if tampered {
			return nil
		}
	} else if !fetchedConfig.Missing() {
		logger.Warn().Str(LogKeyAudit, "issue_comment").Msg("Skipped tampering check because the policy is not valid")
	}

	mbrCtx := NewCrossOrgMembershipContext(ctx, client, repo.GetOwner().GetLogin(), h.Installations, h.ClientCreator)
	return h.EvaluateFetchedConfig(ctx, mbrCtx, client, pr, fetchedConfig)
}

func (h *IssueComment) detectAndLogTampering(ctx context.Context, client *github.Client, event github.IssueCommentEvent, pr *github.PullRequest, config *policy.Config) (bool, error) {
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

		repo := pr.GetBase().GetRepo()
		srcSHA := pr.GetHead().GetSHA()

		s := h.MakeStatus("failure", msg, nil)
		_, _, err := client.Repositories.CreateStatus(ctx, repo.GetOwner().GetLogin(), repo.GetName(), srcSHA, s)
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
