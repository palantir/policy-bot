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

import (
	"context"
	"testing"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/assert"
)

func TestHasValidSignatures(t *testing.T) {
	pTrue := HasValidSignatures(true)
	pFalse := HasValidSignatures(false)

	testCases := []SignatureTestCase{
		{
			"ValidSignature",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "ttest",
							State:   "VALID",
							KeyID:   "3AA5C34371567BD2",
						},
					},
				},
			},
		},
		{
			"InvalidSignature",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: false,
							Signer:  "ttest",
							State:   "INVALID",
							KeyID:   "3AA5C34371567BD2",
						},
					},
				},
			},
		},
		{
			"NoSignature",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
						Signature: nil,
					},
				},
			},
		},
	}

	runSignatureTests(t, pTrue, testCases)

	// Invert the expected outcomes and test against the false predicate
	for idx := range testCases {
		testCases[idx].Expected = !testCases[idx].Expected
	}
	runSignatureTests(t, pFalse, testCases)
}

func TestHasValidSignaturesBy(t *testing.T) {
	p := &HasValidSignaturesBy{
		common.Actors{
			Teams:         []string{"testorg/team"},
			Users:         []string{"mhaypenny"},
			Organizations: []string{"testorg"},
		},
	}

	runSignatureTests(t, p, []SignatureTestCase{
		{
			"ValidSignatureByUser",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "mhaypenny",
							State:   "VALID",
							KeyID:   "3AA5C34371567BD2",
						},
					},
				},
			},
		},
		{
			"ValidSignatureButNotUser",
			false,
			&pulltest.Context{
				AuthorValue: "badcommitter",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "badcommitter",
						Committer: "badcommitter",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "badcommitter",
							State:   "VALID",
							KeyID:   "3AD5C34671567BC3",
						},
					},
				},
			},
		},
		{
			"ValidSignatureByTeamMember",
			true,
			&pulltest.Context{
				AuthorValue: "ttest",
				TeamMemberships: map[string][]string{
					"ttest": {
						"testorg/team",
					},
				},
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "ttest",
							State:   "VALID",
							KeyID:   "3AA5C34371567BD2",
						},
					},
				},
			},
		},
		{
			"NoSignature",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "mhaypenny",
						Committer: "mhaypenny",
						Signature: nil,
					},
				},
			},
		},
	})
}

func TestHasValidSignaturesByKeys(t *testing.T) {
	p := &HasValidSignaturesByKeys{
		KeyIDs: []string{"3AA5C34371567BD2"},
	}

	runSignatureTests(t, p, []SignatureTestCase{
		{
			"ValidSignatureByValidKey",
			true,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "mhaypenny",
							State:   "VALID",
							KeyID:   "3AA5C34371567BD2",
						},
					},
				},
			},
		},
		{
			"ValidSignatureByInvalidKey",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: true,
							Signer:  "mhaypenny",
							State:   "VALID",
							KeyID:   "3AB5C35371567CE7",
						},
					},
				},
			},
		},
		{
			"InvalidSignatureByInvalidKey",
			false,
			&pulltest.Context{
				AuthorValue: "mhaypenny",
				CommitsValue: []*pull.Commit{
					{
						SHA:       "abcdef123456789",
						Author:    "ttest",
						Committer: "ttest",
						Signature: &pull.Signature{
							Type:    pull.SignatureGpg,
							IsValid: false,
							Signer:  "mhaypenny",
							State:   "BAD_EMAIL",
							KeyID:   "3AB5C35371567CE7",
						},
					},
				},
			},
		},
	})
}

type SignatureTestCase struct {
	Name     string
	Expected bool
	Context  pull.Context
}

func runSignatureTests(t *testing.T, p Predicate, cases []SignatureTestCase) {
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ok, _, _, err := p.Evaluate(ctx, tc.Context)
			if assert.NoError(t, err, "evaluation failed") {
				assert.Equal(t, tc.Expected, ok, "predicate was not correct")
			}
		})
	}
}
