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
	"fmt"

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

	// Disqualifications contains candidates whose approvals were not counted
	// towards this rule, along with the reason for each.
	Disqualifications []*Disqualification

	// Conditions contains the results of all required conditions
	Conditions []*PredicateResult
}

// DisqualificationReason is why a candidate's approval was not counted.
type DisqualificationReason string

const (
	// DisqualifiedAuthor means the candidate opened the pull request and the
	// rule does not set allow_author.
	DisqualifiedAuthor DisqualificationReason = "author"

	// DisqualifiedContributor means the candidate contributed a commit to the
	// pull request and the rule does not set allow_contributor or
	// allow_non_author_contributor.
	DisqualifiedContributor DisqualificationReason = "contributor"

	// DisqualifiedNotRequired means the candidate is not one of the users,
	// organizations, teams, or permission holders that the rule requires.
	DisqualifiedNotRequired DisqualificationReason = "not-required"
)

// Disqualification records an approval that was not counted towards a rule and
// the reason it was not counted.
type Disqualification struct {
	Candidate *Candidate
	Reason    DisqualificationReason

	// Commit is the full SHA of the commit that made the candidate a
	// contributor. It is only set when Reason is DisqualifiedContributor.
	Commit string
}

// Description explains the disqualification to a human reading the details
// page.
func (d *Disqualification) Description() string {
	switch d.Reason {
	case DisqualifiedAuthor:
		return "Opened this pull request"
	case DisqualifiedContributor:
		if d.Commit != "" {
			return fmt.Sprintf("Contributed commit %.7s", d.Commit)
		}
		return "Contributed a commit to this pull request"
	case DisqualifiedNotRequired:
		return "Not one of the users this rule requires"
	}
	return string(d.Reason)
}

type Dismissal struct {
	Candidate *Candidate
	Reason    string
}
