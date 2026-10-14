# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`policy-bot` is a GitHub App that evaluates a repository's `.policy.yml` against a pull
request and reports the result as a commit status. Everything it does is driven by webhooks:
an event arrives, the policy is evaluated against a fresh view of the pull request, and a
status is posted. There is no database of approvals and no polling loop — the current state
is always recomputed from GitHub.

`develop` is the integration branch; PRs target it.

## Commands

The build is driven by [godel](https://github.com/palantir/godel) via the `./godelw` wrapper.

```bash
./godelw verify --apply=false   # format check + mod + license + lint + tests (what CI runs)
./godelw verify                 # same, but fixes formatting and license headers in place
./godelw format                 # apply the formatter only
./godelw test                   # tests only
./godelw run policy-bot server  # run the server against config/policy-bot.yml
```

`verify --apply=false` is the exact CI gate (`.github/workflows/build.yml`). Run it before
pushing.

Plain `go test ./policy/...` and `go test ./pull/ -run TestX -v` work fine for a fast inner
loop, but they do **not** check formatting. The formatter is `ptimports`, not plain `gofmt`:
it enforces import grouping (stdlib, blank line, everything else) and will fail `verify` on a
file that `gofmt -l` considers clean. When adding a new file, run `./godelw format` before
`verify`.

Every source file needs the Apache license header; `./godelw verify` adds it.

## Architecture

### Evaluation flow

1. A webhook arrives at a handler in `server/handler/` (`pull_request.go`, `issue_comment.go`,
   `pull_request_review.go`, `status.go`, `check_run.go`, `workflow_run.go`, `merge_group.go`).
   These are the *only* events policy-bot sees.
2. The handler builds an `EvalContext` (`server/handler/eval_context.go`) and calls
   `ParseConfig(ctx, trigger)` with the `common.Trigger` that the event corresponds to.
3. `policy.ParsePolicy` (`policy/policy.go`) turns the parsed `.policy.yml` into a tree of
   `common.Evaluator`s: approval rules combined by `and`/`or`/`if`, plus the disapproval policy.
4. Evaluation produces a `common.Result` tree, which becomes the commit status and the
   `/details` page.

### `pull.Context` is the GitHub data layer

`pull/context.go` defines the interface; `pull/github.go` implements it against the v3 REST
and v4 GraphQL APIs, caching every field on the struct so a single evaluation fetches each
thing at most once. There are two other implementations: `pull/pulltest.Context` (a plain
struct of `XxxValue`/`XxxError` fields — add a field and a method when you extend the
interface) and `policy/simulated.Context`, which embeds `pull.Context` and so picks up new
methods for free.

**`loadPagedData` is cost-tuned.** One GraphQL query fetches commits, comments and reviews for
one rate-limit point, and the comment above it says to verify the cost before changing page
sizes or adding fields. Prefer a separate, lazily-loaded REST call for data that only some
policies need — see `Labels()` and `Reactions()` — so policies that don't use it pay nothing.

### Triggers are an optimization, and a correctness constraint

`common.Trigger` (`policy/common/trigger.go`) is a bit set of the event kinds that could
change a rule's value. Each rule ORs together the triggers of its methods and predicates, and
an event whose trigger doesn't intersect the rule's is skipped. If you add a signal, you must
extend `Rule.Trigger()` in `policy/approval/approve.go` (and `Policy.Trigger()` in
`policy/disapproval/disapprove.go`), or the rule silently won't re-evaluate.

If a signal has no corresponding webhook at all, `TriggerAll` is the only correct answer —
see the reactions method, and `#### Approval by Reaction` in the README for why.

### Approval methods and candidates

`policy/common/methods.go` is where a signal becomes an approval. `Methods.Candidates()`
turns comments, PR body, reviews and reactions into `[]*Candidate{Type, User, CreatedAt,
LastEditedAt}`, deduplicated to the most recent action per user. `policy/approval/approve.go`
then filters candidates (`ignore_edited_comments` drops any with a non-zero `LastEditedAt`;
`invalidate_on_push` drops any created before the last push) and matches the survivors
against `requires.users` / `organizations` / `teams`.

`Methods` has a three-level defaults chain — rule, then policy `approval_defaults`, then
server config — implemented by the `Defaults *Methods` pointer and the `GetXxx()` accessors.
Always read options through the accessor, never the field: the field being `nil` means
"inherit", while an explicit empty list means "off".

## Conventions

**Options accessors.** Every option is a pointer or slice with a `GetXxx()`/`IsXxx()` reader
that walks the defaults chain:

```go
func (m *Methods) GetComments() []string {
	if m.Comments == nil {
		if m.Defaults != nil {
			return m.Defaults.GetComments()
		}
		return nil
	}
	return m.Comments
}
```

**Validate config at parse time.** A typo in `.policy.yml` should be an error the user sees,
not a rule that silently never matches. Give the field a named type with an `UnmarshalYAML`
method — see `common.Regexp` and `common.ReactionContent`.

**Errors** use `github.com/pkg/errors` with `Wrapf` and context about what failed.

**Tests** use `testify` (`require` to stop, `assert` to continue) and table/subtest style.
HTTP-backed code in `pull/` is tested with `ResponsePlayer`, which replays YAML fixtures from
`pull/testdata/responses/`; a fixture is a list of responses, returned in order, so multiple
entries plus a `Link` header exercise pagination. `rule.Count` from `AddRule` asserts how many
requests were actually made, which is how caching gets tested.

**Docs live in `README.md`**, which is the user-facing spec for `.policy.yml`. A new option
needs an entry in the commented YAML block *and* a line in the table of contents (it is
hand-maintained; headings marked `<!-- omit in toc -->` are deliberately excluded).
