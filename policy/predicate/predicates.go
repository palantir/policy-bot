// Copyright 2021 Palantir Technologies, Inc.
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

package predicate

type Predicates struct {
	ChangedFiles     *ChangedFiles     `yaml:"changed_files"`
	OnlyChangedFiles *OnlyChangedFiles `yaml:"only_changed_files"`

	HasAuthorIn             *HasAuthorIn             `yaml:"has_author_in"`
	HasContributorIn        *HasContributorIn        `yaml:"has_contributor_in"`
	OnlyHasContributorsIn   *OnlyHasContributorsIn   `yaml:"only_has_contributors_in"`
	AuthorIsOnlyContributor *AuthorIsOnlyContributor `yaml:"author_is_only_contributor"`

	TargetsBranch *TargetsBranch `yaml:"targets_branch"`
	FromBranch    *FromBranch    `yaml:"from_branch"`

	ModifiedLines *ModifiedLines `yaml:"modified_lines"`

	HasSuccessfulStatus *HasSuccessfulStatus `yaml:"has_successful_status"`

	HasLabels *HasLabels `yaml:"has_labels"`

	MatchesTitlePattern *MatchesTitlePattern `yaml:"title"`
}

func (p *Predicates) Predicates() []Predicate {
	var ps []Predicate

	if p.ChangedFiles != nil {
		ps = append(ps, Predicate(p.ChangedFiles))
	}
	if p.OnlyChangedFiles != nil {
		ps = append(ps, Predicate(p.OnlyChangedFiles))
	}

	if p.HasAuthorIn != nil {
		ps = append(ps, Predicate(p.HasAuthorIn))
	}
	if p.HasContributorIn != nil {
		ps = append(ps, Predicate(p.HasContributorIn))
	}
	if p.OnlyHasContributorsIn != nil {
		ps = append(ps, Predicate(p.OnlyHasContributorsIn))
	}
	if p.AuthorIsOnlyContributor != nil {
		ps = append(ps, Predicate(p.AuthorIsOnlyContributor))
	}

	if p.TargetsBranch != nil {
		ps = append(ps, Predicate(p.TargetsBranch))
	}
	if p.FromBranch != nil {
		ps = append(ps, Predicate(p.FromBranch))
	}

	if p.ModifiedLines != nil {
		ps = append(ps, Predicate(p.ModifiedLines))
	}

	if p.HasSuccessfulStatus != nil {
		ps = append(ps, Predicate(p.HasSuccessfulStatus))
	}

	if p.HasLabels != nil {
		ps = append(ps, Predicate(p.HasLabels))
	}

	if p.MatchesTitlePattern != nil {
		ps = append(ps, Predicate(p.MatchesTitlePattern))
	}

	return ps
}
