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

type AdminScope string

const (
	User AdminScope = "user"
	Team AdminScope = "team"
	Org  AdminScope = "org"
)

type ReviewRequestRule struct {
	Teams              []string
	Users              []string
	Organizations      []string
	Admins             bool
	WriteCollaborators bool
	RequiredCount      int
	AdminScope         AdminScope
}

type Result struct {
	Name              string
	Description       string
	Status            EvaluationStatus
	ReviewRequestRule ReviewRequestRule

	Error error

	Children []*Result
}
