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

package disapproval

import (
	"context"
	"fmt"
	"sort"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Policy struct {
	Predicates predicate.Predicates `yaml:"if"`
	Options    Options              `yaml:"options"`
	Requires   Requires             `yaml:"requires"`
}

type Options struct {
	Methods Methods `yaml:"methods"`
}

type Methods struct {
	Disapprove *common.Methods `yaml:"disapprove"`
	Revoke     *common.Methods `yaml:"revoke"`
}

func (opts *Options) GetDisapproveMethods() *common.Methods {
	m := opts.Methods.Disapprove
	if m == nil {
		githubReview := true
		comments := []string{
			":-1:",
			"👎",
		}
		m = &common.Methods{
			Comments:     &comments,
			GithubReview: &githubReview,
		}
	}

	m.GithubReviewState = pull.ReviewChangesRequested
	return m
}

func (opts *Options) GetRevokeMethods() *common.Methods {
	m := opts.Methods.Revoke
	if m == nil {
		githubReview := true
		comments := []string{
			":+1:",
			"👍",
		}
		m = &common.Methods{
			Comments:     &comments,
			GithubReview: &githubReview,
		}
	}

	m.GithubReviewState = pull.ReviewApproved
	return m
}

type Requires struct {
	common.Actors `yaml:",inline"`
}

func (p *Policy) Trigger() common.Trigger {
	t := common.TriggerCommit

	if !p.Requires.IsEmpty() {
		dm := p.Options.GetDisapproveMethods()
		rm := p.Options.GetRevokeMethods()

		if dm.Comments != nil && len(*dm.Comments) > 0 || rm.Comments != nil && len(*rm.Comments) > 0 {
			t |= common.TriggerComment
		}
		if dm.GithubReview != nil && *dm.GithubReview || rm.GithubReview != nil && *rm.GithubReview {
			t |= common.TriggerReview
		}
	}

	for _, predicate := range p.Predicates.Predicates() {
		t |= predicate.Trigger()
	}

	return t
}

func (p *Policy) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	log := zerolog.Ctx(ctx)

	res.Name = "disapproval"
	res.Status = common.StatusSkipped
	res.Requires = p.Requires.Actors

	var predicateResults []*common.PredicateResult

	for _, p := range p.Predicates.Predicates() {
		result, err := p.Evaluate(ctx, prctx)
		if err != nil {
			res.Error = errors.Wrap(err, "failed to evaluate predicate")
			return
		}
		predicateResults = append(predicateResults, result)

		if result.Satisfied {
			log.Debug().Msgf("disapproving, predicate of type %T was satisfied", p)

			res.Status = common.StatusDisapproved

			desc := result.Description
			res.StatusDescription = desc
			if desc == "" {
				res.StatusDescription = "A precondition of this rule was satisfied"
			}
			res.PredicateResults = []*common.PredicateResult{result}
			return
		}
	}
	res.PredicateResults = predicateResults
	if p.Requires.IsEmpty() {
		log.Debug().Msg("no users are allowed to disapprove; skipping")

		res.StatusDescription = "No disapproval policy is specified or the policy is empty"
		return
	}

	disapproved, msg, err := p.IsDisapproved(ctx, prctx)
	if err != nil {
		res.Error = errors.WithMessage(err, "failed to compute disapproval status")
		return
	}

	res.StatusDescription = msg
	if disapproved {
		res.Status = common.StatusDisapproved
	} else {
		res.Status = common.StatusSkipped
	}
	return
}

func (p *Policy) IsDisapproved(ctx context.Context, prctx pull.Context) (disapproved bool, msg string, err error) {
	disapproveMethods := p.Options.GetDisapproveMethods()
	revokeMethods := p.Options.GetRevokeMethods()

	disapprover, err := p.lastActor(ctx, prctx, disapproveMethods, "disapproval")
	if err != nil {
		return false, "", errors.WithMessage(err, "failed to get last disapprover")
	}

	// exit early if there is no disapprover
	if disapprover == nil {
		msg = "No disapprovals"
		return
	}

	revoker, err := p.lastActor(ctx, prctx, revokeMethods, "revocation")
	if err != nil {
		return false, "", errors.WithMessage(err, "failed to get last revoker")
	}

	switch {
	// someone disapproved, but nobody has revoked
	case revoker == nil:
		disapproved = true
		msg = fmt.Sprintf("Disapproved by %s", disapprover.User)

	// the new disapproval appears after a revocation
	case disapprover.CreatedAt.After(revoker.CreatedAt):
		disapproved = true
		msg = fmt.Sprintf("Disapproved by %s", disapprover.User)

	// a disapproval has been revoked
	default:
		msg = fmt.Sprintf("Disapproval revoked by %s", revoker.User)
	}
	return
}

func (p *Policy) lastActor(ctx context.Context, prctx pull.Context, methods *common.Methods, kind string) (*common.Candidate, error) {
	log := zerolog.Ctx(ctx)

	candidates, err := methods.Candidates(ctx, prctx)
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("found %d %s candidates", len(candidates), kind)

	candidates, err = p.filter(ctx, prctx, candidates)
	if err != nil {
		return nil, err
	}

	sort.Stable(common.CandidatesByCreationTime(candidates))

	return last(candidates), nil
}

func (p *Policy) filter(ctx context.Context, prctx pull.Context, candidates []*common.Candidate) ([]*common.Candidate, error) {
	log := zerolog.Ctx(ctx)

	var filtered []*common.Candidate
	for _, c := range candidates {
		ok, err := p.Requires.IsActor(ctx, prctx, c.User)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to check candidate status")
		}

		if !ok {
			log.Debug().Str("user", c.User).Msg("ignoring disapproval/revocation by non-whitelisted user")
			continue
		}

		filtered = append(filtered, c)
	}
	return filtered, nil
}

func last(c []*common.Candidate) *common.Candidate {
	if len(c) == 0 {
		return nil
	}
	return c[len(c)-1]
}
