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

	"github.com/pkg/errors"

	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
)

type HasValidSignatures bool

var _ Predicate = HasValidSignatures(false)

func (pred HasValidSignatures) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to get commits")
	}

	for _, c := range commits {
		if c.Signature == nil {
			return false, fmt.Sprintf("Commit %.10s has no signature", c.SHA), nil
		}
		if !c.Signature.IsValid {
			return false, fmt.Sprintf("Commit %.10s has an invalid signaturer due to %s", c.SHA, c.Signature.State), nil
		}
	}

	return true, "", nil
}

func (pred HasValidSignatures) Trigger() common.Trigger {
	return common.TriggerCommit
}

type HasValidSignaturesBy struct {
	common.Actors `yaml:",inline"`
}

var _ Predicate = &HasValidSignaturesBy{}

func (pred *HasValidSignaturesBy) Evaluate(ctx context.Context, prctx pull.Context) (bool, string, error) {
	commits, err := prctx.Commits()
	if err != nil {
		return false, "", errors.Wrap(err, "failed to get commits")
	}

	signers := make(map[string]struct{})

	for _, c := range commits {
		if c.Signature == nil {
			return false, fmt.Sprintf("Commit %.10s has no signature", c.SHA), nil
		}
		if !c.Signature.IsValid {
			return false, fmt.Sprintf("Commit %.10s has an invalid signaturer", c.SHA), nil
		}
		signers[c.Signature.Signer] = struct{}{}
	}

	for signer := range signers {
		member, err := pred.IsActor(ctx, prctx, signer)
		if err != nil {
			return false, "", err
		}
		if !member {
			return false, fmt.Sprintf("Contributor %q does not meet the required membership conditions for signing", signer), nil
		}
	}

	return true, "", nil
}

func (pred *HasValidSignaturesBy) Trigger() common.Trigger {
	return common.TriggerCommit
}
