// Copyright 2025 Palantir Technologies, Inc.
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

package server

import (
	"testing"
)

func TestTemplateGitHubAPIPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// /repos/{owner}/{repo}/* paths
		{"/repos/example-org/example-repo/pulls/12345/files", "/repos/{owner}/{repo}/pulls/{pull_number}/files"},
		{"/repos/example-org/example-repo/pulls/12345/reviews", "/repos/{owner}/{repo}/pulls/{pull_number}/reviews"},
		{"/repos/example-org/example-repo/issues/12345/comments", "/repos/{owner}/{repo}/issues/{issue_number}/comments"},
		{"/repos/example-org/example-repo/commits/abcdef0123456789abcdef0123456789abcdef01/statuses", "/repos/{owner}/{repo}/commits/{sha}/statuses"},
		{"/repos/example-org/example-repo/commits/abcdef0123456789abcdef0123456789abcdef01/check-runs", "/repos/{owner}/{repo}/commits/{sha}/check-runs"},
		{"/repos/example-org/example-repo/statuses/abcdef0123456789abcdef0123456789abcdef01", "/repos/{owner}/{repo}/statuses/{sha}"},
		{"/repos/example-org/example-repo/check-runs", "/repos/{owner}/{repo}/check-runs"},
		{"/repos/example-org/example-repo/check-runs/67890", "/repos/{owner}/{repo}/check-runs/{check_run_id}"},
		{"/repos/example-org/example-repo/actions/runs/789012", "/repos/{owner}/{repo}/actions/runs/{run_id}"},

		// SHA-eating paths
		{"/repos/example-org/example-repo/git/trees/abcdef0123456789abcdef0123456789abcdef01", "/repos/{owner}/{repo}/git/trees/{sha}"},
		{"/repos/example-org/example-repo/git/blobs/abc1234", "/repos/{owner}/{repo}/git/blobs/{sha}"},

		// branches & refs (ref values can contain slashes)
		{"/repos/example-org/example-repo/branches/main", "/repos/{owner}/{repo}/branches/{branch}"},
		{"/repos/example-org/example-repo/git/refs/heads/feature/big-feature", "/repos/{owner}/{repo}/git/refs/heads/{ref}"},

		// non-repo paths
		{"/app/installations/123456789/access_tokens", "/app/installations/{installation_id}/access_tokens"},
		{"/users/alice", "/users/{username}"},
		{"/orgs/example-org/members/alice", "/orgs/{org}/members/{username}"},
		{"/orgs/example-org/teams/platform/members/alice", "/orgs/{org}/teams/{team_slug}/members/{username}"},
		{"/repositories/123456", "/repositories/{repository_id}"},

		// no-template paths pass through
		{"/rate_limit", "/rate_limit"},
		{"/", "/"},
		{"", "/"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := templateGitHubAPIPath(tc.in)
			if got != tc.want {
				t.Errorf("templateGitHubAPIPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
