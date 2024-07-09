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
	KeyIDs []string `yaml:"key_ids,omitempty"`
}

var _ Predicate = &HasValidSignaturesByKeys{}

func (pred *HasValidSignaturesByKeys) Evaluate(ctx context.Context, prctx pull.Context) (*common.PredicateResult, error) {
	commits, err := prctx.Commits()

	predicateResult := common.PredicateResult{
		ConditionPhrase: "have valid signatures by keys",
		ConditionValues: pred.KeyIDs,
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to get commits")
	}

	keys := make(map[string][]string)

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
		// Only GPG signatures are valid for this predicate
		switch c.Signature.Type {
		case pull.SignatureGpg:
			commitSHAs, ok := keys[c.Signature.KeyID]
			if ok {
				keys[c.Signature.KeyID] = append(commitSHAs, c.SHA)
			} else {
				keys[c.Signature.KeyID] = []string{c.SHA}
			}
		default:
			predicateResult.Values = []string{c.SHA}
			predicateResult.ValuePhrase = "commits"
			predicateResult.ConditionPhrase = "have GPG signatures"
			predicateResult.Description = fmt.Sprintf("Commit %.10s signature is not a GPG signature", c.SHA)
			predicateResult.Satisfied = false
			return &predicateResult, nil
		}
	}

	for key := range keys {
		isValidKey := false
		for _, acceptedKey := range pred.KeyIDs {
			if key == acceptedKey {
				isValidKey = true
				break
			}
		}
		if !isValidKey {
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
