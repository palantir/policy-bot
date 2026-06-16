// Copyright 2024 Palantir Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommitUnattributedContributor(t *testing.T) {
	tests := map[string]struct {
		commit  Commit
		wantOK  bool
		wantDsc string
	}{
		"bothAttributed": {
			commit: Commit{Author: "alice", Committer: "bob"},
			wantOK: false,
		},
		"unattributedAuthorWithNameAndEmail": {
			commit: Commit{
				Author:      "",
				AuthorName:  "Mallory",
				AuthorEmail: "mallory@noreply.invalid",
				Committer:   "bob",
			},
			wantOK:  true,
			wantDsc: "author Mallory <mallory@noreply.invalid>",
		},
		"unattributedAuthorWithEmailOnly": {
			commit: Commit{
				Author:      "",
				AuthorEmail: "mallory@noreply.invalid",
				Committer:   "bob",
			},
			wantOK:  true,
			wantDsc: "author <mallory@noreply.invalid>",
		},
		"unattributedAuthorWithNoIdentity": {
			commit:  Commit{Author: "", Committer: "bob"},
			wantOK:  true,
			wantDsc: "author unknown",
		},
		"unattributedCommitter": {
			commit: Commit{
				Author:        "alice",
				Committer:     "",
				CommitterName: "Mallory",
			},
			wantOK:  true,
			wantDsc: "committer Mallory",
		},
		"unattributedCommitterViaWebIsExempt": {
			commit: Commit{Author: "alice", Committer: "", CommittedViaWeb: true},
			wantOK: false,
		},
		"unattributedAuthorTakesPrecedenceOverCommitter": {
			commit: Commit{
				Author:      "",
				AuthorEmail: "mallory@noreply.invalid",
				Committer:   "",
			},
			wantOK:  true,
			wantDsc: "author <mallory@noreply.invalid>",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			desc, ok := test.commit.UnattributedContributor()
			assert.Equal(t, test.wantOK, ok, "UnattributedContributor ok mismatch")
			assert.Equal(t, test.wantOK, test.commit.HasUnattributedContributor(), "HasUnattributedContributor mismatch")
			if test.wantOK {
				assert.Equal(t, test.wantDsc, desc)
			}
		})
	}
}
