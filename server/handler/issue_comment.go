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

	"github.com/google/go-github/v59/github"
	"github.com/palantir/go-githubapp/githubapp"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
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

	pr, _, err := client.PullRequests.Get(ctx, owner, repo.GetName(), number)
	if err != nil {
		return errors.Wrapf(err, "failed to get pull request %s/%s#%d", owner, repo.GetName(), number)
	}

	ctx, logger := h.PreparePRContext(ctx, installationID, pr)

	evalCtx, err := h.NewEvalContext(ctx, installationID, pull.Locator{
		Owner:  owner,
		Repo:   repo.GetName(),
		Number: number,
		Value:  pr,
	})
	if err != nil {
		return err
	}

	switch {
	case evalCtx.Config.LoadError != nil || evalCtx.Config.ParseError != nil:
		logger.Warn().Str(LogKeyAudit, "issue_comment").Msg("Skipping tampering check because the policy is not valid")
	case evalCtx.Config.Config != nil:
		tampered := h.detectAndLogTampering(ctx, evalCtx, event)
		if tampered {
			return nil
		}
	}

	evaluator, err := evalCtx.ParseConfig(ctx, common.TriggerComment)
	if err != nil {
		return err
	}
	if evaluator == nil {
		return nil
	}

	if !h.affectsApproval(event, evalCtx.Config.Config) {
		logger.Debug().Msg("Skipping evaluation because this comment does not impact approval")
		return nil
	}

	result, err := evalCtx.EvaluatePolicy(ctx, evaluator)
	if err != nil {
		return err
	}

	evalCtx.RunPostEvaluateActions(ctx, result, common.TriggerComment)
	return nil
}

func (h *IssueComment) detectAndLogTampering(ctx context.Context, evalCtx *EvalContext, event github.IssueCommentEvent) bool {
	logger := zerolog.Ctx(ctx)

	if event.GetAction() == "created" {
		return false
	}

	eventAuthor := event.GetSender().GetLogin()
	commentAuthor := event.GetComment().GetUser().GetLogin()
	if eventAuthor == commentAuthor {
		return false
	}

	if h.affectsApproval(event, evalCtx.Config.Config) {
		msg := fmt.Sprintf("Entity %s edited approval comment by %s", eventAuthor, commentAuthor)
		logger.Warn().Str(LogKeyAudit, "issue_comment").Msg(msg)

		evalCtx.PostStatus(ctx, "failure", msg)
		return true
	}

	logger.Warn().Str(LogKeyAudit, "issue_comment").Msgf("The comment_editor=%s is not the author=%s", eventAuthor, commentAuthor)
	return true
}

func (h *IssueComment) affectsApproval(event github.IssueCommentEvent, config *policy.Config) bool {
	var body, originalBody string
	switch event.GetAction() {
	case "edited":
		body = event.GetComment().GetBody()
		originalBody = event.GetChanges().GetBody().GetFrom()
	default:
		body = event.GetComment().GetBody()
		originalBody = body
	}

	var methods []*common.Methods
	for _, rule := range config.ApprovalRules {
		methods = append(methods, rule.Options.GetMethods())
	}
	if disapproval := config.Policy.Disapproval; disapproval != nil {
		methods = append(methods, disapproval.Options.GetDisapproveMethods())
		methods = append(methods, disapproval.Options.GetRevokeMethods())
	}

	for _, m := range methods {
		if m.CommentMatches(body) || (body != originalBody && m.CommentMatches(originalBody)) {
			return true
		}
	}
	return false
}
