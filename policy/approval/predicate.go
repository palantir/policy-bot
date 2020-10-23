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

package approval

import (
	"github.com/palantir/policy-bot/policy/predicate"
)

type Predicates struct {
	ChangedFiles     *predicate.ChangedFiles     `yaml:"changed_files"`
	OnlyChangedFiles *predicate.OnlyChangedFiles `yaml:"only_changed_files"`

	HasAuthorIn             *predicate.HasAuthorIn             `yaml:"has_author_in"`
	HasContributorIn        *predicate.HasContributorIn        `yaml:"has_contributor_in"`
	OnlyHasContributorsIn   *predicate.OnlyHasContributorsIn   `yaml:"only_has_contributors_in"`
	AuthorIsOnlyContributor *predicate.AuthorIsOnlyContributor `yaml:"author_is_only_contributor"`

	TargetsBranch *predicate.TargetsBranch `yaml:"targets_branch"`
	FromBranch    *predicate.FromBranch    `yaml:"from_branch"`

	ModifiedLines *predicate.ModifiedLines `yaml:"modified_lines"`

	HasSuccessfulStatus *predicate.HasSuccessfulStatus `yaml:"has_successful_status"`

	HasLabels *predicate.HasLabels `yaml:"has_labels"`
}

func (p *Predicates) Predicates() []predicate.Predicate {
	var ps []predicate.Predicate

	if p.ChangedFiles != nil {
		ps = append(ps, predicate.Predicate(p.ChangedFiles))
	}
	if p.OnlyChangedFiles != nil {
		ps = append(ps, predicate.Predicate(p.OnlyChangedFiles))
	}

	if p.HasAuthorIn != nil {
		ps = append(ps, predicate.Predicate(p.HasAuthorIn))
	}
	if p.HasContributorIn != nil {
		ps = append(ps, predicate.Predicate(p.HasContributorIn))
	}
	if p.OnlyHasContributorsIn != nil {
		ps = append(ps, predicate.Predicate(p.OnlyHasContributorsIn))
	}
	if p.AuthorIsOnlyContributor != nil {
		ps = append(ps, predicate.Predicate(p.AuthorIsOnlyContributor))
	}

	if p.TargetsBranch != nil {
		ps = append(ps, predicate.Predicate(p.TargetsBranch))
	}
	if p.FromBranch != nil {
		ps = append(ps, predicate.Predicate(p.FromBranch))
	}

	if p.ModifiedLines != nil {
		ps = append(ps, predicate.Predicate(p.ModifiedLines))
	}

	if p.HasSuccessfulStatus != nil {
		ps = append(ps, predicate.Predicate(p.HasSuccessfulStatus))
	}

	if p.HasLabels != nil {
		ps = append(ps, predicate.Predicate(p.HasLabels))
	}

	return ps
}
