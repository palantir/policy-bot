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

package policy

import (
	"context"

	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/disapproval"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// Deprecated: RemoteConfig allows the use of a remote policy file, rather than a local one. The Remote value
// should follow the format `org/repo`. An example: `palantir/policy-bot`. The Path is optional,
// with the default value being the configured default policy file location. The Ref is optional,
// and the default branch of the Remote repository will be used.
type RemoteConfig struct {
	Remote string `yaml:"remote"`
	Path   string `yaml:"path"`
	Ref    string `yaml:"ref"`
}

type Config struct {
	Policy        Policy           `yaml:"policy"`
	ApprovalRules []*approval.Rule `yaml:"approval_rules"`
}

type Policy struct {
	Approval    approval.Policy     `yaml:"approval"`
	Disapproval *disapproval.Policy `yaml:"disapproval"`
}

func ParsePolicy(c *Config) (common.Evaluator, error) {
	rulesByName := make(map[string]*approval.Rule)
	for _, r := range c.ApprovalRules {
		rulesByName[r.Name] = r
	}

	evalApproval, err := c.Policy.Approval.Parse(rulesByName)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse approval policy")
	}

	evalDisapproval := c.Policy.Disapproval
	if evalDisapproval == nil {
		evalDisapproval = &disapproval.Policy{}
	}

	return evaluator{
		approval:    evalApproval,
		disapproval: evalDisapproval,
	}, nil
}

type evaluator struct {
	approval    common.Evaluator
	disapproval common.Evaluator
}

func (e evaluator) Trigger() common.Trigger {
	return e.approval.Trigger() | e.disapproval.Trigger()
}

func (e evaluator) Evaluate(ctx context.Context, prctx pull.Context) (res common.Result) {
	disapproval := e.disapproval.Evaluate(ctx, prctx)
	approval := e.approval.Evaluate(ctx, prctx)

	res.Name = "policy"
	res.Children = []*common.Result{&approval, &disapproval}

	for _, r := range res.Children {
		if r.Error != nil {
			res.Error = r.Error
		}
	}

	switch {
	case res.Error != nil:
	case disapproval.Status == common.StatusDisapproved:
		res.Status = common.StatusDisapproved
		res.StatusDescription = disapproval.StatusDescription
	default:
		res.Status = approval.Status
		res.StatusDescription = approval.StatusDescription
	}
	return
}
