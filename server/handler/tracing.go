// Copyright 2025 Palantir Technologies, Inc.
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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/palantir/policy-bot/server/handler"

// Attribute keys used across spans. Following the convention `<domain>.<field>`
// so dashboards can filter on a flat namespace (github.*, policy.*).
const (
	AttrEventType      = "github.event.type"
	AttrEventAction    = "github.event.action"
	AttrDeliveryID     = "github.delivery_id"
	AttrInstallationID = "github.installation_id"
	AttrRepoOwner      = "github.repo.owner"
	AttrRepoName       = "github.repo.name"
	AttrRepoFull       = "github.repo.full_name"
	AttrPRNumber       = "github.pr.number"
	AttrSHA            = "github.sha"
	AttrReviewState    = "github.review.state"
	AttrReviewer       = "github.review.author"
	AttrCheckRunName   = "github.check_run.name"
	AttrCheckRunStatus = "github.check_run.conclusion"
	AttrSenderLogin    = "github.sender.login"

	AttrPolicyStatus        = "policy.status"
	AttrPolicyTrigger       = "policy.trigger"
	AttrPolicyPolicyTrigger = "policy.policy_trigger"
	AttrPolicyRulesApproved = "policy.rules_approved"
	AttrPolicyRulesTotal    = "policy.rules_total"
	AttrPolicySkipReason    = "policy.skip_reason"
	AttrPolicyConfigSource  = "policy.config_source"
	AttrPolicyConfigPath    = "policy.config_path"

	// Debounce attributes describe how StatusDebouncer coalesced an event.
	// They are set on the webhook span so a debounced delivery is identifiable
	// on its own, and on the trailing-evaluation span.
	AttrDebounceKey                = "policy.debounce.key"
	AttrDebounceDecision           = "policy.debounce.decision"
	AttrDebounceReason             = "policy.debounce.reason"
	AttrDebounceTrailing           = "policy.debounce.trailing"
	AttrDebounceTrailingScheduled  = "policy.debounce.trailing_scheduled"
	AttrDebounceTrailingDelayMs    = "policy.debounce.trailing_delay_ms"
	AttrDebounceTrailGen           = "policy.debounce.trail_gen"
	AttrDebounceWindowMs           = "policy.debounce.window_ms"
	AttrDebounceAccumulatedTrigger = "policy.debounce.accumulated_trigger"
	AttrDebounceCoalesced          = "policy.debounce.coalesced_count"
)

// Debounce decision values for AttrDebounceDecision. Keep stable — dashboard
// filter values.
const (
	DebounceDecisionEvaluate = "evaluate_immediately"
	DebounceDecisionSkip     = "skip_debounced"
)

// Debounce reason values for AttrDebounceReason explaining the decision.
const (
	DebounceReasonFirstEvent    = "first_event"
	DebounceReasonWindowExpired = "window_expired"
	DebounceReasonWithinWindow  = "within_window"
)

// Skip-reason values for AttrPolicySkipReason. Keep stable — these are
// dashboard/alert filter values.
const (
	SkipReasonSelfSender                   = "self_sender"
	SkipReasonSelfCheckRun                 = "self_check_run"
	SkipReasonNoPolicy                     = "no_policy"
	SkipReasonNoPolicyConfig               = "no_policy_config"
	SkipReasonTriggerMismatch              = "trigger_mismatch"
	SkipReasonReviewDoesNotAffectApproval  = "review_does_not_affect_approval"
	SkipReasonCommentDoesNotAffectApproval = "comment_does_not_affect_approval"
	SkipReasonActionNotHandled             = "action_not_handled"
	SkipReasonNotPullRequest               = "not_pull_request"
	SkipReasonDifferentRepo                = "pr_from_different_repo"
	SkipReasonInstallationActionNotHandled = "installation_action_not_handled"
	SkipReasonStatusContextNotHandled      = "status_context_not_handled"
	SkipReasonStatusStatePending           = "status_state_pending"
	SkipReasonInvalidPolicyTamperingCheck  = "invalid_policy_during_tampering_check"
	SkipReasonCommentTampered              = "comment_tampered"
	SkipReasonAllEvaluationsSucceeded      = "all_evaluations_succeeded"
)

// Tracer returns the package tracer.
func Tracer() trace.Tracer { return otel.Tracer(tracerName) }

// StartWebhookSpan opens a span for a webhook handler invocation. Span name
// is `webhook.<event_type>` so Jaeger searches like `operation=webhook.pull_request_review`
// surface every delivery of that event. Always returns a non-nil span; safe to
// call when OTEL is disabled (the global no-op tracer takes over).
func StartWebhookSpan(ctx context.Context, eventType, deliveryID string) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "webhook."+eventType,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String(AttrEventType, eventType),
			attribute.String(AttrDeliveryID, deliveryID),
		),
	)
}

// StartChildSpan opens a child span with no extra attributes. Use it for the
// internal evaluation phases (policy.parse_config, policy.evaluate_policy, ...).
func StartChildSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name)
}

// StartDebounceTrailingSpan opens a span for a debounced trailing evaluation
// that runs after the originating webhook handler has already returned. The
// request context is canceled by the time the trailing timer fires, so this
// detaches cancellation while preserving trace linkage and the logger.
func StartDebounceTrailingSpan(ctx context.Context) (context.Context, trace.Span) {
	return Tracer().Start(context.WithoutCancel(ctx), "policy.debounce_trailing",
		trace.WithAttributes(attribute.Bool(AttrDebounceTrailing, true)),
	)
}

// RecordError marks the span as failed and attaches the error. Pass nil to
// no-op — useful so callers can write `defer RecordError(span, &err)`.
func RecordError(span trace.Span, errPtr *error) {
	if errPtr == nil || *errPtr == nil {
		return
	}
	span.RecordError(*errPtr)
	span.SetStatus(codes.Error, (*errPtr).Error())
}

// RepoAttrs returns the standard owner/name/full_name attributes for a repo.
func RepoAttrs(owner, name string) []attribute.KeyValue {
	full := owner + "/" + name
	return []attribute.KeyValue{
		attribute.String(AttrRepoOwner, owner),
		attribute.String(AttrRepoName, name),
		attribute.String(AttrRepoFull, full),
	}
}
