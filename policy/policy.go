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
	"maps"
	"slices"

	"github.com/palantir/go-githubapp/appconfig"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/disapproval"
	"github.com/palantir/policy-bot/pull"
	"github.com/pkg/errors"
)

// RemoteConfig allows the use of a remote policy file, rather than a local one. The Remote value
// should follow the format `org/repo`. An example: `palantir/policy-bot`. The Path is optional,
// with the default value being the configured default policy file location. The Ref is optional,
// and the default branch of the Remote repository will be used.
type RemoteConfig struct {
	Remote string `yaml:"remote,omitempty"`
	Path   string `yaml:"path,omitempty"`
	Ref    string `yaml:"ref,omitempty"`
}

type Config struct {
	Policy           Policy             `yaml:"policy,omitempty"`
	ApprovalDefaults *approval.Defaults `yaml:"approval_defaults,omitempty"`
	ApprovalRules    []*approval.Rule   `yaml:"approval_rules,omitempty"`
	Slots            Slots              `yaml:"slots,omitempty"`
}

type Slots map[string]approval.Policy

type Policy struct {
	Approval        approval.Policy      `yaml:"approval,omitempty"`
	ApprovalExtends *appconfig.RemoteRef `yaml:"approval_extends,omitempty"`
	Disapproval     *disapproval.Policy  `yaml:"disapproval,omitempty"`
}

// GlobalOptions defines server-level properties that affect policy parsing, like default or override values.
type GlobalOptions struct {
	// IgnoreEditedComments, if non-nil, overrides the value of this option in
	// all rules. If true, editing a comment will invalidate the comment for
	// approval in all policies.
	IgnoreEditedComments *bool

	// ApprovalDefaults defines server-level default values for policies. For
	// instance, this can change the default approval strings for all policies.
	ApprovalDefaults *approval.Defaults
}

func ParsePolicy(c *Config, opts *GlobalOptions) (common.Evaluator, error) {
	file, err := Parse(c, opts)
	if err != nil {
		return nil, err
	}
	parsed, err := file.Resolve()
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

type ParsedPolicy struct {
	evaluator
	methods []*common.Methods
}

// Methods returns the methods used for event filtering and comment tampering
// detection.
func (p *ParsedPolicy) Methods() []*common.Methods {
	return p.methods
}

func (p *ParsedPolicy) FindApprovalRule(slot, name string) *approval.Rule {
	return p.approval.FindRule(slot, name)
}

// File is a parsed policy file. Call Resolve to prepare it for evaluation.
type File struct {
	approval     approval.Evaluator
	slots        map[string]*approval.SlotRule
	slotContents map[string]approval.Evaluator
	rules        []*approval.Rule
	extends      *appconfig.RemoteRef
	disapproval  *disapproval.Policy
}

// Parse validates one file using its own rules and defaults without requiring
// complete slot content or a resolved parent.
func Parse(c *Config, opts *GlobalOptions) (*File, error) {
	if c.Policy.ApprovalExtends != nil {
		if c.Policy.Approval != nil {
			return nil, errors.New("approval and approval_extends cannot both be specified")
		}
		if _, _, err := c.Policy.ApprovalExtends.SplitRemote(); err != nil {
			return nil, errors.WithMessage(err, "invalid approval_extends")
		}
	}
	rules, err := prepareRules(c, opts)
	if err != nil {
		return nil, err
	}
	evalApproval, slots, err := c.Policy.Approval.Parse(rules)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse approval policy")
	}
	contents := make(map[string]approval.Evaluator, len(c.Slots))
	for name, slotContent := range c.Slots {
		if name == "" {
			return nil, errors.New("slot name must be nonempty")
		}
		slotEvaluator, err := slotContent.ParseAsSlotContent(rules)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse slot content %q", name)
		}
		if slots[name] == nil && c.Policy.ApprovalExtends == nil {
			return nil, errors.Errorf("slot content %q does not match a declared slot", name)
		}
		contents[name] = slotEvaluator
	}
	return &File{
		approval:     evalApproval,
		slots:        slots,
		slotContents: contents,
		rules:        c.ApprovalRules,
		extends:      c.Policy.ApprovalExtends,
		disapproval:  c.Policy.Disapproval,
	}, nil
}

func (f *File) Extends() *appconfig.RemoteRef {
	return f.extends
}

// Extend combines the parent and child, using the child's slot content and
// disapproval policy. It panics if the child has no extension reference.
// Call Resolve to validate the result.
func (parent *File) Extend(child *File) *File {
	if child.extends == nil {
		panic("child policy must have approval_extends")
	}
	contents := maps.Clone(parent.slotContents)
	maps.Copy(contents, child.slotContents)
	return &File{
		approval:     parent.approval,
		slots:        parent.slots,
		slotContents: contents,
		rules:        slices.Concat(parent.rules, child.rules),
		extends:      parent.extends,
		disapproval:  child.disapproval,
	}
}

// Resolve attaches slot content and validates the completed policy.
func (f *File) Resolve() (*ParsedPolicy, error) {
	if f.extends != nil {
		return nil, errors.New("approval_extends requires a resolved parent; use authenticated simulation for full validation")
	}
	for name, content := range f.slotContents {
		if slot := f.slots[name]; slot != nil {
			slot.Child = content
		}
	}
	var missing []string
	for name, slot := range f.slots {
		if slot.Required && slot.Child == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, errors.Errorf("required slots are empty: %q", missing)
	}
	parsed := &ParsedPolicy{}
	for _, rule := range f.rules {
		parsed.methods = append(parsed.methods, rule.Options.GetMethods())
	}
	evalDisapproval := f.disapproval
	if evalDisapproval == nil {
		evalDisapproval = &disapproval.Policy{}
	} else {
		parsed.methods = append(parsed.methods,
			evalDisapproval.Options.GetDisapproveMethods(), evalDisapproval.Options.GetRevokeMethods())
	}
	parsed.evaluator = evaluator{approval: f.approval, disapproval: evalDisapproval}
	return parsed, nil
}

func prepareRules(c *Config, opts *GlobalOptions) (map[string]*approval.Rule, error) {
	// Build the options hierarchy in reverse order. When reading an option,
	// values are tried in the following order:
	//
	// 1. Rule
	// 2. Policy default
	// 3. Configurable server default
	// 4. Hardcoded server default
	//
	defaultApprovalOptions := &approval.Options{
		Methods: approval.DefaultMethods(),
	}
	if opts != nil && opts.ApprovalDefaults != nil {
		defaultApprovalOptions = setDefaultOptions(defaultApprovalOptions, opts.ApprovalDefaults.Options)
	}
	if c.ApprovalDefaults != nil {
		defaultApprovalOptions = setDefaultOptions(defaultApprovalOptions, c.ApprovalDefaults.Options)
	}

	rulesByName := make(map[string]*approval.Rule)
	for _, r := range c.ApprovalRules {
		if r == nil {
			return nil, errors.New("approval rule must not be null")
		}
		if _, exists := rulesByName[r.Name]; exists {
			return nil, errors.Errorf("duplicate approval rule name %q", r.Name)
		}

		// Set policy and server rule defaults
		r.Options.Defaults = defaultApprovalOptions

		// Override rule options controlled by the server
		if opts != nil {
			if opts.IgnoreEditedComments != nil {
				r.Options.IgnoreEditedComments = opts.IgnoreEditedComments
			}
		}

		rulesByName[r.Name] = r
	}

	return rulesByName, nil
}

func setDefaultOptions(existing *approval.Options, next *approval.Options) *approval.Options {
	if next != nil {
		next.Defaults = existing
		return next
	}
	return existing
}

type evaluator struct {
	approval    approval.Evaluator
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
