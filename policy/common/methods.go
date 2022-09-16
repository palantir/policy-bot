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

	// If GithubReview is true, GithubReviewState is the state a review must
	// have to be considered a candidated. It is currently excluded from
	// serialized forms and should be set by the application.
	GithubReviewState pull.ReviewState `yaml:"-" json:"-"`
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

	if len(m.Comments) > 0 || len(m.CommentPatterns) > 0 {
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

	if m.GithubReview != nil && *m.GithubReview || len(m.GithubReviewCommentPatterns) > 0 {
		reviews, err := prctx.Reviews()
		if err != nil {
			return nil, err
		}

		for _, r := range reviews {
			if r.State == m.GithubReviewState {
				if len(m.GithubReviewCommentPatterns) > 0 {
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
	for _, comment := range m.Comments {
		if strings.Contains(commentBody, comment) {
			return true
		}
	}
	for _, pattern := range m.CommentPatterns {
		if pattern.Matches(commentBody) {
			return true
		}
	}
	return false
}

func (m *Methods) GithubReviewCommentMatches(commentBody string) bool {
	for _, pattern := range m.GithubReviewCommentPatterns {
		if pattern.Matches(commentBody) {
			return true
		}
	}
	return false
}
