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
	"context"
	"strings"
	"time"

	"github.com/palantir/policy-bot/pull"
)

type Methods struct {
	Comments                    []string `yaml:"comments,omitempty"`
	CommentPatterns             []Regexp `yaml:"comment_patterns,omitempty"`
	GithubReview                *bool    `yaml:"github_review,omitempty"`
	GithubReviewCommentPatterns []Regexp `yaml:"github_review_comment_patterns,omitempty"`
	BodyPatterns                []Regexp `yaml:"body_patterns,omitempty"`

	// If GithubReview is true, GithubReviewState is the state a review must
	// have to be considered a candidate. It is set after parsing based on the
	// context in which Methods is used, rather than by the YAML configuration.
	GithubReviewState pull.ReviewState `yaml:"-"`
}

func (m *Methods) GetComments() []string {
	return m.Comments
}

func (m *Methods) GetCommentPatterns() []Regexp {
	return m.CommentPatterns
}

func (m *Methods) IsGithubReview() bool {
	if m.GithubReview == nil {
		return false
	}
	return *m.GithubReview
}

func (m *Methods) GetGithubReviewCommentPatterns() []Regexp {
	return m.GithubReviewCommentPatterns
}

func (m *Methods) GetBodyPatterns() []Regexp {
	return m.BodyPatterns
}

type CandidateType string

const (
	ReviewCandidate  CandidateType = "review"
	CommentCandidate CandidateType = "comment"
)

type Candidate struct {
	Type         CandidateType
	ReviewID     string
	User         string
	CreatedAt    time.Time
	LastEditedAt time.Time
}

type CandidatesByCreationTime []*Candidate

func (cs CandidatesByCreationTime) Len() int      { return len(cs) }
func (cs CandidatesByCreationTime) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs CandidatesByCreationTime) Less(i, j int) bool {
	return cs[i].CreatedAt.Before(cs[j].CreatedAt)
}

// Candidates returns a list of user candidates based on the configured
// methods. A given user will appear at most once in the list. If that user has
// taken multiple actions that match the methods, only the most recent by event
// order is included. The order of the candidates is unspecified.
func (m *Methods) Candidates(ctx context.Context, prctx pull.Context) ([]*Candidate, error) {
	var candidates []*Candidate

	if len(m.GetComments()) > 0 || len(m.GetCommentPatterns()) > 0 {
		comments, err := prctx.Comments()
		if err != nil {
			return nil, err
		}

		for _, c := range comments {
			if m.CommentMatches(c.Body) {
				candidates = append(candidates, &Candidate{
					Type:         CommentCandidate,
					User:         c.Author,
					CreatedAt:    c.CreatedAt,
					LastEditedAt: c.LastEditedAt,
				})
			}
		}
	}

	if len(m.GetBodyPatterns()) > 0 {
		prBody, err := prctx.Body()
		if err != nil {
			return nil, err
		}
		if m.BodyMatches(prBody.Body) {
			candidates = append(candidates, &Candidate{
				User:         prBody.Author,
				CreatedAt:    prBody.CreatedAt,
				LastEditedAt: prBody.LastEditedAt,
			})
		}
	}

	if m.IsGithubReview() || len(m.GetGithubReviewCommentPatterns()) > 0 {
		reviews, err := prctx.Reviews()
		if err != nil {
			return nil, err
		}

		for _, r := range reviews {
			if r.State == m.GithubReviewState {
				if len(m.GetGithubReviewCommentPatterns()) > 0 {
					if m.GithubReviewCommentMatches(r.Body) {
						candidates = append(candidates, &Candidate{
							Type:         ReviewCandidate,
							ReviewID:     r.ID,
							User:         r.Author,
							CreatedAt:    r.CreatedAt,
							LastEditedAt: r.LastEditedAt,
						})
					}
				} else {
					candidates = append(candidates, &Candidate{
						Type:         ReviewCandidate,
						ReviewID:     r.ID,
						User:         r.Author,
						CreatedAt:    r.CreatedAt,
						LastEditedAt: r.LastEditedAt,
					})
				}
			}
		}
	}

	return deduplicateCandidates(candidates), nil
}

func deduplicateCandidates(all []*Candidate) []*Candidate {
	users := make(map[string]*Candidate)
	for _, c := range all {
		last, ok := users[c.User]
		if !ok || last.CreatedAt.Before(c.CreatedAt) {
			users[c.User] = c
		}
	}

	candidates := make([]*Candidate, 0, len(users))
	for _, c := range users {
		candidates = append(candidates, c)
	}

	return candidates
}

func (m *Methods) CommentMatches(commentBody string) bool {
	for _, comment := range m.GetComments() {
		if strings.Contains(commentBody, comment) {
			return true
		}
	}
	for _, pattern := range m.GetCommentPatterns() {
		if pattern.Matches(commentBody) {
			return true
		}
	}
	return false
}

func (m *Methods) GithubReviewCommentMatches(commentBody string) bool {
	for _, pattern := range m.GetGithubReviewCommentPatterns() {
		if pattern.Matches(commentBody) {
			return true
		}
	}
	return false
}

func (m *Methods) BodyMatches(prBody string) bool {
	for _, pattern := range m.GetBodyPatterns() {
		if pattern.Matches(prBody) {
			return true
		}
	}
	return false
}
