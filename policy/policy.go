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

func (s *Slots) UnmarshalYAML(unmarshal func(any) error) error {
	var fills map[any]approval.Policy
	if err := unmarshal(&fills); err != nil {
		return err
	}
	result := make(Slots, len(fills))
	for key, fill := range fills {
		name, ok := key.(string)
		if !ok || name == "" {
			return errors.New("slot fill name must be a nonempty string")
		}
		result[name] = fill
	}
	*s = result
	return nil
}

type Policy struct {
	Approval        approval.Policy      `yaml:"approval,omitempty"`
	ApprovalExtends *appconfig.RemoteRef `yaml:"approval_extends,omitempty"`
	Disapproval     *disapproval.Policy  `yaml:"disapproval,omitempty"`

	// Root-level YAML merges can decode this struct more than once. Retain
	// explicit null approval keys so a later extension cannot hide their presence.
	approvalSet bool
}

func (p *Policy) UnmarshalYAML(unmarshal func(any) error) error {
	type plain Policy
	decoded := plain(*p)
	if err := unmarshal(&decoded); err != nil {
		return err
	}

	// Typed decoding loses the distinction between an absent key and null.
	// Use a struct to retain the same merge behavior as the typed decoding.
	fields := struct {
		Approval        yamlFieldPresence `yaml:"approval"`
		ApprovalExtends yamlFieldPresence `yaml:"approval_extends"`
		Disapproval     yamlFieldPresence `yaml:"disapproval"`
	}{
		Approval:        yamlFieldPresence{absent: true},
		ApprovalExtends: yamlFieldPresence{absent: true},
	}
	if err := unmarshal(&fields); err != nil {
		return err
	}
	decoded.approvalSet = decoded.approvalSet || !fields.Approval.absent
	if !fields.ApprovalExtends.absent {
		if decoded.ApprovalExtends == nil {
			return errors.New("approval_extends must be a non-null remote reference")
		}
		if _, _, err := decoded.ApprovalExtends.SplitRemote(); err != nil {
			return errors.WithMessage(err, "invalid approval_extends")
		}
	}
	if decoded.approvalSet && decoded.ApprovalExtends != nil {
		return errors.New("approval and approval_extends cannot both be specified")
	}
	*p = Policy(decoded)
	return nil
}

// Both a decoded null and a call to UnmarshalYAML clear absent. Ignoring the
// contents here is safe because the typed decoder above validates them first.
type yamlFieldPresence struct {
	absent bool
}

func (p *yamlFieldPresence) UnmarshalYAML(_ func(any) error) error {
	p.absent = false
	return nil
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
	parsed, err := ParsePolicyWithParent(c, nil, opts)
	if err != nil {
		return nil, err
	}
	return parsed.evaluator, nil
}

// ParsedPolicy provides the evaluator, event methods, and rule lookup for one
// resolved policy. Slot rule lookup uses the file that supplied the selected fill.
type ParsedPolicy struct {
	evaluator
	baseRules map[string]*approval.Rule
	slotRules map[string]map[string]*approval.Rule
	methods   []*common.Methods
}

func (p *ParsedPolicy) Methods() []*common.Methods {
	return p.methods
}

func (p *ParsedPolicy) FindApprovalRule(slot, name string) *approval.Rule {
	if slot == "" {
		return p.baseRules[name]
	}
	return p.slotRules[slot][name]
}

// ParsePolicyWithParent composes a child's fills with its literal parent's
// approval tree. Rules and defaults remain local to their defining file, and
// only the child's disapproval policy applies.
func ParsePolicyWithParent(child, parent *Config, opts *GlobalOptions) (*ParsedPolicy, error) {
	base := child
	if child.Policy.ApprovalExtends != nil {
		if child.Policy.approvalSet || child.Policy.Approval != nil {
			return nil, errors.New("approval and approval_extends cannot both be specified")
		}
		if _, _, err := child.Policy.ApprovalExtends.SplitRemote(); err != nil {
			return nil, errors.WithMessage(err, "invalid approval_extends")
		}
		if parent == nil {
			return nil, errors.New("approval_extends requires a resolved parent; use authenticated simulation for full validation")
		}
		if parent.Policy.ApprovalExtends != nil {
			return nil, errors.New("an approval parent cannot specify approval_extends")
		}
		base = parent
	} else if parent != nil {
		return nil, errors.New("a parent was supplied without approval_extends")
	}

	baseRules, err := prepareRules(base, opts)
	if err != nil {
		return nil, err
	}
	slots := make(map[string]*approval.SlotRule)
	evalApproval, err := base.Policy.Approval.Parse(baseRules, slots)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse approval policy")
	}
	parsed := &ParsedPolicy{
		baseRules: baseRules,
		slotRules: make(map[string]map[string]*approval.Rule),
	}
	if err := parsed.applyFills(base, baseRules, slots); err != nil {
		return nil, err
	}
	if parent != nil {
		childRules, err := prepareRules(child, opts)
		if err != nil {
			return nil, err
		}
		if err := parsed.applyFills(child, childRules, slots); err != nil {
			return nil, err
		}
	}
	var missing []string
	for name, slot := range slots {
		if slot.Required && slot.Child == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, errors.Errorf("required slots are empty: %q", missing)
	}

	rules := child.ApprovalRules
	if parent != nil {
		rules = approval.Rules(evalApproval)
	}
	for _, rule := range rules {
		parsed.methods = append(parsed.methods, rule.Options.GetMethods())
	}
	evalDisapproval := child.Policy.Disapproval
	if evalDisapproval == nil {
		evalDisapproval = &disapproval.Policy{}
	} else {
		parsed.methods = append(parsed.methods,
			evalDisapproval.Options.GetDisapproveMethods(),
			evalDisapproval.Options.GetRevokeMethods())
	}
	parsed.evaluator = evaluator{approval: evalApproval, disapproval: evalDisapproval}
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
	for i, r := range c.ApprovalRules {
		if r == nil {
			return nil, errors.Errorf("approval rule at index %d must not be null", i)
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

func (p *ParsedPolicy) applyFills(c *Config, rulesByName map[string]*approval.Rule, slots map[string]*approval.SlotRule) error {
	for name, fill := range c.Slots {
		if name == "" || fill == nil {
			return errors.Errorf("slot fill %q must have a nonempty name and a non-null list of rules", name)
		}
		child, err := fill.ParseFill(rulesByName)
		if err != nil {
			return errors.Wrapf(err, "failed to parse slot fill %q", name)
		}
		if slot := slots[name]; slot != nil {
			slot.Child = child
			if child == nil {
				delete(p.slotRules, name)
			} else {
				p.slotRules[name] = rulesByName
			}
		}
	}
	return nil
}

func setDefaultOptions(existing *approval.Options, next *approval.Options) *approval.Options {
	if next != nil {
		options := *next
		options.Defaults = existing
		return &options
	}
	return existing
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
