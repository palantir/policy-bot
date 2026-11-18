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
	"fmt"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

type HasValidSignatures bool

var _ Predicate = HasValidSignatures(false)

func (pred HasValidSignatures) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ConditionPhrase: "have",
		ConditionValues: []string{"valid signatures"},
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		commitHashes = append(commitHashes, c.SHA)
		if !valid {
			predicateResult.Values = []string{c.SHA}
			if pred {
				predicateResult.Description = desc
				predicateResult.Satisfied = false
				return &predicateResult, nil
			}
			predicateResult.Satisfied = true
			return &predicateResult, nil
		}
	}
	predicateResult.Values = commitHashes
	if pred {
		predicateResult.Satisfied = true
		return &predicateResult, nil
	}
	predicateResult.Satisfied = false
	predicateResult.Description = "All commits are signed and have valid signatures"
	return &predicateResult, nil
}

func (pred HasValidSignatures) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasValidSignaturesBy struct {
	common.Actors `yaml:",inline,omitempty"`
}

var _ Predicate = &HasValidSignaturesBy{}

func (pred *HasValidSignaturesBy) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ConditionsMap: map[string][]string{
			"Organizations": pred.Organizations,
			"Teams":         pred.Teams,
			"Users":         pred.Users,
		},
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	signers := make(map[string]string)
	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		if !valid {
			predicateResult.ConditionPhrase = "have valid signatures by members of"
			predicateResult.ValuePhrase = "commits"
			predicateResult.Values = []string{c.SHA}
			predicateResult.Description = desc
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
		signers[c.Signature.Signer] = c.SHA
		commitHashes = append(commitHashes, c.SHA)
	}

	var signerList []string

	for signer := range signers {
		signerList = append(signerList, signer)
		member, err := pred.IsActor(ctx, prctx, signer)
		if err != nil {
			return nil, err
		}
		if !member {
			predicateResult.ConditionPhrase = "satisfy the required membership conditions"
			predicateResult.Values = []string{signer}
			predicateResult.ValuePhrase = "signers"
			predicateResult.Description = fmt.Sprintf("Contributor %q does not meet the required membership conditions for signing", signer)
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}
	predicateResult.ConditionPhrase = "have valid signatures by members of"
	predicateResult.Values = commitHashes
	predicateResult.ValuePhrase = "commits"
	predicateResult.Satisfied = true
	return &predicateResult, nil
}

func (pred *HasValidSignaturesBy) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasValidSignaturesByKeys struct {
	KeyIDs          []string `yaml:"key_ids,omitempty"`
	KeyFingerprints []string `yaml:"key_fingerprints,omitempty"`
}

var _ Predicate = &HasValidSignaturesByKeys{}

func (pred *HasValidSignaturesByKeys) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ConditionPhrase: "have valid signatures by keys",
		ConditionValues: pred.allowedKeys(),
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		if !valid {
			predicateResult.Values = []string{c.SHA}
			predicateResult.Description = desc
			predicateResult.ValuePhrase = "commits"
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
		commitHashes = append(commitHashes, c.SHA)

		// GPG signatures are matched by key ID and SSH signatures by key
		// fingerprint. Other signature types (S/MIME) are not supported.
		var key string
		var allowed bool
		switch c.Signature.Type {
		case pull.SignatureGpg:
			key = c.Signature.KeyID
			allowed = slices.Contains(pred.KeyIDs, key)
		case pull.SignatureSSH:
			key = c.Signature.KeyFingerprint
			allowed = slices.ContainsFunc(pred.KeyFingerprints, func(f string) bool {
				return normalizeSSHFingerprint(f) == normalizeSSHFingerprint(key)
			})
		default:
			predicateResult.Values = []string{c.SHA}
			predicateResult.ValuePhrase = "commits"
			predicateResult.ConditionPhrase = "have GPG or SSH signatures"
			predicateResult.Description = fmt.Sprintf("Commit %.10s signature is not a GPG or SSH signature", c.SHA)
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}

		if !allowed {
			predicateResult.ConditionPhrase = "exist in the set of allowed keys"
			predicateResult.Values = []string{key}
			predicateResult.ValuePhrase = "keys"
			predicateResult.Description = fmt.Sprintf("Key %q does not meet the required key conditions for signing", key)
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	predicateResult.Values = commitHashes
	predicateResult.ValuePhrase = "commits"
	predicateResult.Satisfied = true

	return &predicateResult, nil
}

func (pred *HasValidSignaturesByKeys) Trigger() common.Trigger {
	return common.TriggerCommit
}

// allowedKeys returns the configured GPG key IDs followed by the configured
// SSH key fingerprints, for display in predicate results.
func (pred *HasValidSignaturesByKeys) allowedKeys() []string {
	if len(pred.KeyFingerprints) == 0 {
		return pred.KeyIDs
	}
	keys := make([]string, 0, len(pred.KeyIDs)+len(pred.KeyFingerprints))
	keys = append(keys, pred.KeyIDs...)
	return append(keys, pred.KeyFingerprints...)
}

// normalizeSSHFingerprint removes the "SHA256:" prefix that ssh-keygen -l
// prints, so that fingerprints copied from ssh-keygen match the unprefixed
// form that GitHub reports for SSH commit signatures.
func normalizeSSHFingerprint(fingerprint string) string {
	return strings.TrimPrefix(strings.TrimSpace(fingerprint), "SHA256:")
}

func hasValidSignature(ctx context.Context, commit *pull.Commit) (bool, string) {
	if commit.Signature == nil {
		return false, fmt.Sprintf("Commit %.10s has no signature", commit.SHA)
	}
	if !commit.Signature.IsValid {
		reason := commit.Signature.State
		return false, fmt.Sprintf("Commit %.10s has an invalid signature due to %s", commit.SHA, reason)
	}
	return true, ""
}
