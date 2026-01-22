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

package common

import (
	"github.com/palantir/policy-bot/pull"
)

type EvaluationStatus int

const (
	StatusSkipped EvaluationStatus = iota // note: values used for ordering
	StatusPending
	StatusApproved
	StatusDisapproved
)

func (s EvaluationStatus) String() string {
	switch s {
	case StatusSkipped:
		return "skipped"
	case StatusPending:
		return "pending"
	case StatusApproved:
		return "approved"
	case StatusDisapproved:
		return "disapproved"
	}
	return "unknown"
}

type RequestMode string

const (
	RequestModeAllUsers    RequestMode = "all-users"
	RequestModeRandomUsers RequestMode = "random-users"
	RequestModeTeams       RequestMode = "teams"
)

type ReviewRequestRule struct {
	Teams          []string
	Users          []string
	Organizations  []string
	Permissions    []pull.Permission
	Codeowners     bool
	RequiredCount  int
	RequestedCount int

	Mode RequestMode
}

type Result struct {
	Name              string
	Description       string
	StatusDescription string
	Status            EvaluationStatus
	Error             error
	PredicateResults  []*PredicateResult
	Methods           *Methods

	// Requires contains the result of evaluating the rule's
	// requirements.
	Requires RequiresResult

	// Dismissals contains candidates that should be discarded because they
	// cannot satisfy any future evaluations.
	Dismissals []*Dismissal

	ReviewRequestRule *ReviewRequestRule

	Children []*Result
}

type RequiresResult struct {
	// Count is the number of required approvals from Actors
	// Actors is the set of actors allowed to approve
	// Approvers contains the actual approvers found during evalutaion
	Count     int
	Actors    Actors
	Approvers []*Candidate

	// Conditions contains the results of all required conditions
	Conditions []*PredicateResult

	// OwnershipGroups contains the results for each codeowner group when
	// codeowners approval is required. Each group must have at least one
	// approver for the rule to pass.
	OwnershipGroups []OwnershipGroupResult
}

// OwnershipGroupResult contains the approval status for a codeowner group.
type OwnershipGroupResult struct {
	// Key is a deterministic identifier for this ownership group.
	Key string

	// Owners contains the codeowners for this group (e.g., "@team-a", "@user1").
	Owners []string

	// Files contains the file paths that belong to this ownership group.
	Files []string

	// Satisfied is true if at least one approver is a member of this group.
	Satisfied bool

	// Approvers contains the usernames of approvers who are members of this group.
	Approvers []string
}

type Dismissal struct {
	Candidate *Candidate
	Reason    string
}
