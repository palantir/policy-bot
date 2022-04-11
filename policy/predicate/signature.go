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

func (pred HasValidSignatures) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}
	var commitInfo common.CommitInfo

	predicateInfo := common.PredicateInfo{
		Type:       "HasValidSignatures",
		Name:       "Commit Hashes",
		CommitInfo: &commitInfo,
	}

	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		commitHashes = append(commitHashes, c.SHA)
		if !valid {
			commitInfo.CommitHashes = []string{c.SHA}
			if pred {
				return false, desc, &predicateInfo, nil
			}
			return true, "", &predicateInfo, nil
		}
	}
	commitInfo.CommitHashes = commitHashes
	if pred {
		return true, "", &predicateInfo, nil
	}
	return false, "All commits are signed and have valid signatures", &predicateInfo, nil
}

func (pred HasValidSignatures) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasValidSignaturesBy struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasValidSignaturesBy{}

func (pred *HasValidSignaturesBy) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}

	var commitInfo common.CommitInfo

	predicateInfo := common.PredicateInfo{
		Type:       "HasValidSignaturesBy",
		Name:       "Commit Hashes and Signers",
		CommitInfo: &commitInfo,
	}

	commitInfo.Organizations = pred.Organizations
	commitInfo.Teams = pred.Teams
	commitInfo.Users = pred.Users

	signers := make(map[string]string)
	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		if !valid {
			commitInfo.CommitHashes = []string{c.SHA}
			commitInfo.Signers = []string{}
			return false, desc, &predicateInfo, nil
		}
		signers[c.Signature.Signer] = c.SHA
		commitHashes = append(commitHashes, c.SHA)
	}

	var signerList []string

	for signer := range signers {
		signerList = append(signerList, signer)
		member, err := pred.IsActor(ctx, prctx, signer)
		if err != nil {
			return false, "", nil, err
		}
		if !member {
			commitInfo.Signers = []string{signer}
			commitInfo.CommitHashes = []string{signers[signer]}
			return false, fmt.Sprintf("Contributor %q does not meet the required membership conditions for signing", signer), &predicateInfo, nil
		}
	}
	commitInfo.Signers = signerList
	commitInfo.CommitHashes = commitHashes
	return true, "", &predicateInfo, nil
}

func (pred *HasValidSignaturesBy) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasValidSignaturesByKeys struct {
	KeyIDs []string `yaml:"key_ids"`
}

var _ Predicate = &HasValidSignaturesByKeys{}

func (pred *HasValidSignaturesByKeys) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, *common.PredicateInfo, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", nil, errors.Wrap(err, "failed to get commits")
	}

	var commitInfo common.CommitInfo
	commitInfo.RequiredKeys = pred.KeyIDs

	predicateInfo := common.PredicateInfo{
		Type:       "HasValidSignaturesByKeys",
		Name:       "Commit Hashes and Keys",
		CommitInfo: &commitInfo,
	}

	keys := make(map[string][]string)

	var commitHashes []string

	for _, c := range commits {
		valid, desc := hasValidSignature(ctx, c)
		if !valid {
			commitInfo.CommitHashes = []string{c.SHA}
			commitInfo.Keys = []string{}
			return false, desc, &predicateInfo, nil
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
			commitInfo.CommitHashes = []string{c.SHA}
			commitInfo.Keys = []string{c.Signature.KeyID}
			return false, fmt.Sprintf("Commit %.10s signature is not a GPG signature", c.SHA), &predicateInfo, nil
		}
	}

	var keyList []string

	for key := range keys {
		keyList = append(keyList, key)
		isValidKey := false
		for _, acceptedKey := range pred.KeyIDs {
			if key == acceptedKey {
				isValidKey = true
				break
			}
		}
		if !isValidKey {
			commitInfo.CommitHashes = keys[key]
			commitInfo.Keys = []string{key}
			return false, fmt.Sprintf("Key %q does not meet the required key conditions for signing", key), &predicateInfo, nil
		}
	}
	commitInfo.Keys = keyList
	commitInfo.CommitHashes = commitHashes

	return true, "", &predicateInfo, nil
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
