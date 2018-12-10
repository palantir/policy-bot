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
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

var (
	DefaultApproveMethods = common.Methods{
		Comments: []string{
			":+1:",
			"👍",
		},
		GithubReview: true,
	}
)

type Rule struct {
	Name       string     `yaml:"name"`
	Predicates Predicates `yaml:"if"`
	Options    Options    `yaml:"options"`
	Requires   Requires   `yaml:"requires"`
}

type Options struct {
	AllowAuthor      bool `yaml:"allow_author"`
	AllowContributor bool `yaml:"allow_contributor"`
	InvalidateOnPush bool `yaml:"invalidate_on_push"`

	Methods *common.Methods `yaml:"methods"`
}

func (opts *Options) GetMethods() *common.Methods {
	methods := opts.Methods
	if methods == nil {
		methods = &DefaultApproveMethods
	}

	methods.GithubReviewState = pull.ReviewApproved
	return methods
}

type Requires struct {
	Count int `yaml:"count"`

	common.Actors `yaml:",inline"`
}

func (r *Rule) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	log := zerolog.Ctx(ctx)

	res.Name = r.Name
	res.Status = common.StatusSkipped

	for _, p := range r.Predicates.Predicates() {
		satisfied, desc, err := p.Evaluate(ctx, prctx)
		if err != nil {
			res.Error = errors.Wrap(err, "failed to evaluate predicate")
			return
		}

		if !satisfied {
			log.Debug().Msgf("skipping rule, predicate of type %T was not satisfied", p)

			res.Description = desc
			if desc == "" {
				res.Description = "The preconditions of this rule are not satisfied"
			}

			return
		}
	}

	approved, msg, err := r.IsApproved(ctx, prctx)
	if err != nil {
		res.Error = errors.Wrap(err, "failed to compute approval status")
		return
	}

	res.Description = msg
	if approved {
		res.Status = common.StatusApproved
	} else {
		res.Status = common.StatusPending
	}
	return
}

func (r *Rule) IsApproved(ctx context.Context, prctx pull.Context) (bool, string, error) {
	log := zerolog.Ctx(ctx)

	if r.Requires.Count <= 0 {
		log.Debug().Msg("rule requires no approvals")
		return true, "No approval required", nil
	}

	candidates, err := r.Options.GetMethods().Candidates(ctx, prctx)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to get approval candidates")
	}
	sort.Sort(common.CandidatesByModifiedTime(candidates))

	if r.Options.InvalidateOnPush {
		commits, err := prctx.Commits()
		if err != nil {
			return false, "", errors.Wrap(err, "failed to get commits")
		}

		lastCommitOrder := commits[len(commits)-1].Order

		var allowedCandidates []*common.Candidate
		for _, candidate := range candidates {
			if candidate.Order > lastCommitOrder {
				allowedCandidates = append(allowedCandidates, candidate)
			}
		}

		candidates = allowedCandidates
	}

	log.Debug().Msgf("found %d candidates for approval", len(candidates))

	author, err := prctx.Author()
	if err != nil {
		return false, "", err
	}

	// collect users "banned" by approval options
	banned := make(map[string]bool)

	// "author" is the user who opened the PR
	// if contributors are allowed, the author counts as a contributor
	if !r.Options.AllowAuthor && !r.Options.AllowContributor {
		banned[author] = true
	}

	// "contributor" is any user who added a commit to the PR
	if !r.Options.AllowContributor {
		commits, err := prctx.Commits()
		if err != nil {
			return false, "", err
		}

		for _, c := range commits {
			for _, u := range c.Users() {
				if u != author {
					banned[u] = true
				}
			}
		}
	}

	// filter real approvers using banned status and required membership
	var approvers []string
	for _, c := range candidates {
		if banned[c.User] {
			log.Debug().Str("user", c.User).Msg("rejecting approval by banned user")
			continue
		}

		isApprover, err := r.Requires.IsActor(ctx, prctx, c.User)
		if err != nil {
			return false, "", errors.Wrap(err, "failed to check candidate status")
		}
		if !isApprover {
			log.Debug().Str("user", c.User).Msg("ignoring approval by non-whitelisted user")
			continue
		}

		approvers = append(approvers, c.User)
	}

	log.Debug().Msgf("found %d/%d required approvers", len(approvers), r.Requires.Count)
	remaining := r.Requires.Count - len(approvers)

	if remaining <= 0 {
		msg := fmt.Sprintf("Approved by %s", strings.Join(approvers, ", "))
		return true, msg, nil
	}

	if len(candidates) > 0 && len(approvers) == 0 {
		msg := fmt.Sprintf("%d/%d approvals needed. Ignored %s from disqualified users",
			len(approvers),
			r.Requires.Count,
			numberOfApprovals(len(candidates)))
		return false, msg, nil
	}

	msg := fmt.Sprintf("%d/%d approvals required", len(approvers), r.Requires.Count)
	return false, msg, nil
}

func numberOfApprovals(count int) string {
	if count == 1 {
		return "1 approval"
	}

	return fmt.Sprintf("%d approvals", count)
}
