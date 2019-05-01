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

	HasAuthorIn             *predicate.HasAuthorIn      `yaml:"has_author_in"`
	HasContributorIn        *predicate.HasContributorIn `yaml:"has_contributor_in"`
	AuthorIsOnlyContributor bool                        `yaml:"author_is_only_contributor"`

	TargetsBranch *predicate.TargetsBranch `yaml:"targets_branch"`

	ModifiedLines *predicate.ModifiedLines `yaml:"modified_lines"`
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
	if p.AuthorIsOnlyContributor {
		ps = append(ps, predicate.Predicate(&predicate.AuthorIsOnlyContributor{}))
	}

	if p.TargetsBranch != nil {
		ps = append(ps, predicate.Predicate(p.TargetsBranch))
	}
	if p.ModifiedLines != nil {
		ps = append(ps, predicate.Predicate(p.ModifiedLines))
	}

	return ps
}
