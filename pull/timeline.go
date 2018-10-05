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

package pull

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/go-github/github"
	"github.com/google/go-querystring/query"
)

func listIssueTimelineEvents(ctx context.Context, client *github.Client, owner, repo string, number int, opt *github.ListOptions) ([]*pullrequestEvent, *github.Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/%v/timeline", owner, repo, number)
	u, err := addOptions(u, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.mockingbird-preview+json")

	var events []*pullrequestEvent
	resp, err := client.Do(ctx, req, &events)
	return events, resp, err
}

// copied from github.com/go-github/github/github.go
func addOptions(s string, opt *github.ListOptions) (string, error) {
	if opt == nil {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}

type pullrequestEvent struct {
	github.Timeline

	// Commit fields
	Author    *github.CommitAuthor `json:"author,omitempty"`
	Committer *github.CommitAuthor `json:"committer,omitempty"`
	SHA       *string              `json:"sha,omitempty"`

	// Comment + Review fields
	Body *string `json:"body,omitempty"`

	// Review fields
	State       *string      `json:"state,omitempty"`
	User        *github.User `json:"user,omitempty"`
	SubmittedAt *time.Time   `json:"submitted_at,omitempty"`
}

func (e *pullrequestEvent) GetAuthor() *github.CommitAuthor {
	if e == nil || e.Author == nil {
		return nil
	}
	return e.Author
}

func (e *pullrequestEvent) GetCommiter() *github.CommitAuthor {
	if e == nil || e.Committer == nil {
		return nil
	}
	return e.Committer
}

func (e *pullrequestEvent) GetSHA() string {
	if e == nil || e.SHA == nil {
		return ""
	}
	return *e.SHA
}

func (e *pullrequestEvent) GetBody() string {
	if e == nil || e.Body == nil {
		return ""
	}
	return *e.Body
}

func (e *pullrequestEvent) GetState() string {
	if e == nil || e.State == nil {
		return ""
	}
	return *e.State
}

func (e *pullrequestEvent) GetUser() *github.User {
	if e == nil || e.User == nil {
		return nil
	}
	return e.User
}

func (e *pullrequestEvent) GetSubmittedAt() time.Time {
	if e == nil || e.SubmittedAt == nil {
		return time.Time{}
	}
	return *e.SubmittedAt
}
