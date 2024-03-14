# policy-bot <!-- omit in toc -->

[![Docker Pulls](https://img.shields.io/docker/pulls/palantirtechnologies/policy-bot.svg)](https://hub.docker.com/r/palantirtechnologies/policy-bot/)

`policy-bot` is a [GitHub App](https://developer.github.com/apps/) for enforcing
approval policies on pull requests. It does this by creating a status check,
which can be configured as a [required status check][].

While GitHub natively supports [required reviews][], `policy-bot` provides more
complex approval features:

- Require reviews from specific users, organizations, or teams
- Apply rules based on the files, authors, or branches involved in a pull request
- Combine multiple approval rules with `and` and `or` conditions
- Automatically approve pull requests that meet specific conditions

Behavior is configured by a file in each repository. `policy-bot` also provides a
UI to view the detailed approval status of any pull request.

[required status check]: https://help.github.com/articles/enabling-required-status-checks/
[required reviews]: https://help.github.com/articles/about-required-reviews-for-pull-requests/

- [Configuration](#configuration)
  - [policy.yml Specification](#policyyml-specification)
  - [Approval Rules](#approval-rules)
  - [Approval Policies](#approval-policies)
  - [Disapproval Policy](#disapproval-policy)
  - [Testing and Debugging Policies](#testing-and-debugging-policies)
    - [Simulation API](#simulation-api)
  - [Caveats and Notes](#caveats-and-notes)
    - [Disapproval is Disabled by Default](#disapproval-is-disabled-by-default)
    - [Interactions with GitHub Reviews](#interactions-with-github-reviews)
    - [`or`, `and`, and `if` (Rule Predicates)](#or-and-and-if-rule-predicates)
    - [Cross-organization Membership Tests](#cross-organization-membership-tests)
    - [Update Merges](#update-merges)
    - [Automatically Requesting Reviewers](#automatically-requesting-reviewers)
- [Security](#security)
- [Deployment](#deployment)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Configuration

Policies are defined by a `.policy.yml` file at the root of the repository.
You can change this path and file name when running your own instance of the
server.

- The file is read from the most recent commit on the _target_ branch of each
  pull request.

- The file may contain a reference to a policy in a different repository (see
  [Remote Policy Configuration](#remote-policy-configuration).)

- If the file does not exist in the repository, `policy-bot` tries to load a
  shared `policy.yml` file at the root of the `.github` repository in the same
  organization. You can change this path and repository name when running your
  own instance of the server.

- If a policy does not exist in the repository or in the shared organization
  repository, `policy-bot` does not post a status check on the pull request.
  This means it is safe to enable `policy-bot` on all repositories in an
  organization.

### policy.yml Specification

The overall policy is expressed by:

- Lists of rule definitions
- A set of policies that combine the rules or define additional options

Consider the following example, which allows changes to certain paths without
review, but all other changes require review from the `palantir/devtools`.
Any member of the `palantir` organization can also disapprove changes.

```yaml
# the high level policy
policy:
  approval:
    - or:
      - the devtools team has approved
      - only staging files have changed
  disapproval:
    requires:
      organizations:
        - "palantir"

# the list of rules
approval_rules:
  - name: the devtools team has approved
    requires:
      count: 1
      teams:
        - "palantir/devtools"
  - name: only staging files have changed
    if:
      only_changed_files:
        paths:
          - "^staging/.*$"
    requires:
      count: 0
```

#### Notes on YAML Syntax <!-- omit in toc -->

The YAML language specification supports flow scalars (basic values like strings
and numbers) in three formats:
[single-quoted](https://yaml.org/spec/1.2/spec.html#id2788097),
[double-quoted](https://yaml.org/spec/1.2/spec.html#id2787109), and
[plain](https://yaml.org/spec/1.2/spec.html#id2788859). Each support different
escape characters, which can cause confusion when used for regex strings
(which often contain the `\\` character).

- Single Quoted: `'` is used as an escape character. Backslash characters do not need to be escaped.
  e.g. `'^BREAKING CHANGE: (\w| )+$'`
- Double Quoted: `\` is used as an escape character. Backslash characters must
  be escaped with a preceding `\`.
  e.g. `"^BREAKING CHANGE: (\\w| )+$"`
- Plain: There are no escape characters. Backslash characters do not need to be escaped.
  e.g. `^BREAKING CHANGE: (\w| )+$`

#### Remote Policy Configuration <!-- omit in toc -->

You can also define a remote policy by specifying a repository, path, and ref
(only repository is required). Instead of defining a `policy` key, you would
define a `remote` key. Only 1 level of remote configuration is supported by design.

```yaml
# The remote repository to read the policy file from. This is required, and must
# be in the form of "org/repo-name". The policy bot github app must have read
# access to this repository.
remote: org/repo-name

# The path to the policy config file in the remote repository. If none is
# specified, the default path in the server config is used.
path: path/to/policy.yml

# The branch (or tag, or commit hash) that should be used on the remote
# repository. If none is specified, the default branch of the repository is used.
ref: master
```

### Approval Rules

Each list entry in `approval_rules` has the following specification:

```yaml
# "name" is required, and is used to reference rules in the "policy" block
name: "example rule"

# "description" is optional and provides an explanation of the rule or
# additional help for users. Unlike YAML comments, it appears in the pull
# request details UI along with other information about the rule.
description: "A rule that explains how to configure all of the features"

# "if" specifies a set of predicates that must be true for the rule to apply.
# This block, and every condition within it are optional. If the block does not
# exist, the rule applies to every pull request.
if:
  # "changed_files" is satisfied if any file in the pull request matches any
  # regular expression in the "paths" list. If the "ignore" list is present,
  # files in the pull request matching these regular expressions are ignored
  # by this rule.
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  changed_files:
    paths:
      - "^config/.*$"
      - "^server/views/.*\\.tmpl$"
    ignore:
      - "^config/special\\.file$"

  # "only_changed_files" is satisfied if all files changed by the pull request
  # match at least one regular expression in the list.
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  only_changed_files:
    paths:
      - "^config/.*$"

  # "has_author_in" is satisfied if the user who opened the pull request is in
  # the users list or belongs to any of the listed organizations or teams. The
  # `users` field can contain a GitHub App by appending `[bot]` to the end of
  # the name, for example: `fun-github-app[bot]`
  has_author_in:
    users: ["user1", "user2", ...]
    organizations: ["org1", "org2", ...]
    teams: ["org1/team1", "org2/team2", ...]

  # "has_contributor_in" is satisfied if any commits on the pull request have
  # an author or committer in the users list or that belong to any of the
  # listed organizations or teams.
  has_contributor_in:
    users: ["user1", "user2", ...]
    organizations: ["org1", "org2", ...]
    teams: ["org1/team1", "org2/team2", ...]

  # "only_has_contributors_in" is satisfied if all of the commits on the pull
  # request have an author or committer in the users list or that belong to
  # any of the listed organizations or teams.
  only_has_contributors_in:
    users: ["user1", "user2", ...]
    organizations: ["org1", "org2", ...]
    teams: ["org1/team1", "org2/team2", ...]

  # "author_is_only_contributor", when true, is satisfied if all commits in the
  # pull request are authored by and committed by the user who opened the pull
  # request. When false, it is satisfied if at least one commit in the pull
  # request was authored or committed by another user.
  author_is_only_contributor: true

  # "targets_branch" is satisfied if the target branch of the pull request
  # matches the regular expression
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  targets_branch:
    pattern: "^(master|regexPattern)$"

  # "from_branch" is satisfied if the source branch of the pull request
  # matches the regular expression. Note that source branches from forks will
  # have the pattern "repo_owner:branch_name"
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  from_branch:
    pattern: "^(master|regexPattern)$"

  # "modified_lines" is satisfied if the number of lines added or deleted by
  # the pull request matches any of the listed conditions. Each expression is
  # an operator (one of '<', '>' or '='), an optional space, and a number.
  modified_lines:
    additions: "> 100"
    deletions: "> 100"
    total: "> 200"

  # "has_successful_status" is satisfied if the status checks that are specified
  # are marked successful on the head commit of the pull request.
  has_successful_status:
    - "status-name-1"
    - "status-name-2"
    - "status-name-3"

  # "has_labels" is satisfied if the pull request has the specified labels
  # applied
  has_labels:
    - "label-1"
    - "label-2"

  # "repository" is satisfied if the pull request repository matches any one of the
  # patterns within the "matches" list or does not match all of the patterns
  # within the "not_matches" list.
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  repository:
    matches:
      - "^palantir/policy.*$"
    not_matches:
      - "^palantir/.*docs$"

  # "title" is satisfied if the pull request title matches any one of the
  # patterns within the "matches" list or does not match all of the patterns
  # within the "not_matches" list.
  # e.g. this predicate triggers for titles including "BREAKING CHANGE" or titles
  # that are not marked as docs/style/chore changes (using conventional commits
  # formatting)
  #
  # Note: Double-quote strings must escape backslashes while single/plain do not.
  # See the Notes on YAML Syntax section of this README for more information.
  title:
    matches:
      - "^BREAKING CHANGE: (\\w| )+$"
    not_matches:
      - "^(docs|style|chore): (\\w| )+$"

  # "has_valid_signatures" is satisfied if the commits in the pull request
  # all have git commit signatures that have been verified by GitHub
  has_valid_signatures: true

  # "has_valid_signatures_by" is satisfied if the commits in the pull request
  # all have git commit signatures that have been verified by GitHub, and
  # the authenticated signatures are attributed to a user in the users list
  # or belong to a user in any of the listed organizations or teams.
  has_valid_signatures_by:
    users: ["user1", "user2", ...]
    organizations: ["org1", "org2", ...]
    teams: ["org1/team1", "org2/team2", ...]

  # "has_valid_signatures_by_keys" is satisfied if the commits in the pull request
  # all have git commit signatures that have been verified by GitHub, and
  # the authenticated signatures are attributed to a GPG key with an ID in the list.
  has_valid_signatures_by_keys:
    key_ids: ["3AA5C34371567BD2"]

# "options" specifies a set of restrictions on approvals. If the block does not
# exist, the default values are used.
options:
  # If true, approvals by the author of a pull request are considered when
  # calculating the status. False by default.
  allow_author: false

  # If true, the approvals of someone who has committed to the pull request are
  # considered when calculating the status. The pull request author is considered
  # a contributor. If allow_author and allow_contributor would disagree, this option
  # always wins. False by default.
  allow_contributor: false

  # If true, the approvals of someone who has committed to the pull request are
  # considered when calculating the status. In this case, pull request author is NOT
  # considered a contributor. If combined with any combination of allow_author: true
  # or allow_contributors: true, then the pull request author IS considered when
  # calculating approval. False by default.
  allow_non_author_contributor: false

  # If true, pushing new commits to a pull request will invalidate existing
  # approvals for this rule. False by default.
  invalidate_on_push: false

  # If true, comments on PRs, the PR Body, and review comments that have been edited in any way
  # will be ignored when evaluating approval rules. Default is false.
  ignore_edited_comments: false

  # If true, "update merges" do not invalidate approval (if invalidate_on_push
  # is enabled) and their authors/committers do not count as contributors. An
  # "update merge" is a merge commit that was created in the UI or via the API
  # and merges the target branch into the pull request branch. These are
  # commonly created by using the "Update branch" button in the UI.
  ignore_update_merges: false

  # If present, commits authored and committed by users meeting the conditions
  # are ignored for the purposes of approval. This means the users will not
  # count as contributors and their commits will not invalidate approval if
  # invalidate_on_push is enabled. Both the author and the committer must match
  # the conditions to ignore the commit. This option has security implications,
  # see the README for more details.
  ignore_commits_by:
    users: ["bulldozer[bot]"]
    organizations: ["org1"]
    teams: ["org1/team1"]

  # Automatically request reviewers when a Pull Request is opened
  # if this rule is pending, there are no assigned reviewers, and if the
  # Pull Request is not in Draft.
  # Reviewers are selected based on the set of requirements for this rule
  # and reviewers can be augmented using the mode option.
  request_review:
    # False by default
    enabled: true

    # mode modifies how reviewers are selected. `all-users` will request all users
    # who are able to approve the pending rule. `random-users` selects a small
    # set of random users based on the required count of approvals. `teams` will
    # request teams to review. Teams must have explicit access defined under
    # https://github.com/<org>/<repo>/settings/access in order to be tagged,
    # at least until https://github.com/palantir/policy-bot/issues/165 is fixed.
    # Defaults to 'random-users'.
    mode: all-users|random-users|teams
    
    # count sets the number of users requested to review the pull request when
    # using the `random-users` mode. If count is not set or set to 0, request the
    # number of users set by requires.count. Setting this is useful when you want
    # to request more reviewers than the required count. Defaults to 0.
    count: 0

  # "methods" defines how users may express approval.
  methods:
    # If a comment contains a string in this list, it counts as approval. Use
    # the "comment_patterns" option if you want to match full comments. The
    # default values are shown.
    comments:
      - ":+1:"
      - "👍"

    # If a comment matches a regular expression in this list, it counts as
    # approval. Defaults to an empty list.
    #
    # Note: Double-quote strings must escape backslashes while single/plain do not.
    # See the Notes on YAML Syntax section of this README for more information.
    comment_patterns:
      - "^Signed-off by \\s+$"

    # If true, GitHub reviews can be used for approval. All GitHub review approvals
    # will be accepted as approval candidates. Default is true.
    github_review: true

    # Just like the "comment_patterns" option, but for GitHub reviews. Only GitHub
    # review approvals matching the included patterns will be accepted as
    # approval candidates. Defaults to an empty list.
    github_review_comment_patterns:
      - '\b(?i)domain\s*lgtm\b'

    # Just like the "comment_patterns" and "github_review_comment_patterns" option, but
    # for the PR Body description. If a PR body contains a string in this list, it counts as approval. Use
    # the "body_patterns" option if you want to match strings.
    body_patterns:
      - "\b(?i)no-platform"

# "requires" specifies the approval requirements for the rule. If the block
# does not exist, the rule is automatically approved.
requires:
  # "count" is the number of required approvals. The default is 0, meaning no
  # approval is necessary.
  count: 1

  # A user must be in the list of users or belong to at least one of the given
  # organizations or teams for their approval to count for this rule.
  users: ["user1", "user2"]
  organizations: ["org1", "org2"]
  teams: ["org1/team1", "org2/team2"]

  # A user must have at least the minimum permission in this list for their
  # approval to count for this rule. Valid permissions are "admin", "maintain",
  # "write", "triage", and "read".
  #
  # Specifying more than one permission is only useful to control which users
  # or teams are selected for review requests. See the documentation on review
  # requests for details.
  permissions: ["write"]

  # Deprecated: use 'permissions: ["admin"]'
  #
  # Allows approval by admins of the org or repository
  # admins: true

  # Deprecated: use 'permissions: ["write"]'
  #
  # Allows approval by users who have write on the repository
  # write_collaborators: true
```

### Approval Policies

The `approval` block in the `policy` section defines a list of rules that must
all be true:

```yaml
policy:
  approval:
    - rule1
    - rule2
    - rule3
    - ...
```

Each list entry may be the name of a rule, or one of the following
conjunctions:

```yaml
or:
  - rule1
  - rule2
  - ...
```

```yaml
and:
  - rule1
  - rule2
  - ...
```

Conjunctions can contain more conjunctions (up to a maximum depth of 5):

```yaml
- or:
    - rule1
    - rule2
    - and:
        - rule3
        - rule4
```

### Disapproval Policy

Disapproval allows users to explicitly block pull requests if certain changes
must be made. Any member of in the set of allowed users can disapprove a change
or revoke another user's disapproval.

Unlike approval, all disapproval predicates and options are specified as part
of the policy. Effectively, there is a single disapproval rule. The `disapproval`
policy has the following specification:

```yaml
# "disapproval" is the top-level key in the policy block.
disapproval:
  # "if" specifies a set of predicates which will cause disapproval if any are
  # true
  #
  # This block, and every condition within it are optional. If the block does
  # not exist, a pull request is only disapproved if a user takes a disapproval
  # action.
  if:
    # All predicates from the approval rules section are valid here
    title:
      not_matches:
        - "^(fix|feat|chore): (\\w| )+$"
        - "^BREAKING CHANGE: (\\w| )+$"
      matches:
        - "^BLOCKED"

  # "options" sets behavior related to disapproval. If it does not exist, the
  # defaults shown below are used.
  options:
    # "methods" defines how users set and revoke disapproval.
    methods:
      # "disapprove" sets the methods for disapproval.
      disapprove:
        comments:
          - ":-1:"
          - "👎"
        github_review: true

      # "revoke" sets the methods for revoking disapproval. Usually, these will
      # match the methods used by approval rules.
      revoke:
        comments:
          - ":+1:"
          - "👍"
        github_review: true

  # "requires" sets the users that are allowed to disapprove. If it is not set,
  # disapproval is not enabled.
  requires:
    users: ["user1", "user2"]
    organizations: ["org1", "org2"]
    teams: ["org1/team1", "org2/team2"]
```

### Testing and Debugging Policies

Sometimes it is useful to test if a given policy file is valid, especially in a CI environment.

An API endpoint exists at `/api/validate` to validate the syntax of the yaml and policy configuration,
however it cannot validate that the rules are semantically correct for a given use case.

The API can be used as such:

```sh
$ curl https://policybot.domain/api/validate -XPUT -T path/to/policy.yml
{"message":"failed to parse approval policy: failed to parse subpolicies for 'and': policy references undefined rule 'the devtools team has approved', allowed values: [the devtools team has]","version":"1.12.5"}
```

You can examine the HTTP response code to automatically detect failures

```sh
$ rcode=$(curl https://policybot.domain/api/validate -XPUT -T path/to/policy.yml -s -w "%{http_code}" -o /tmp/response)
$ if [[ "${rcode}" -gt 299 ]]; then cat /tmp/response && exit 1; fi
```

#### Simulation API

It can be useful to simulate how Policy Bot would evaluate a pull request if certain conditions were changed. For example: adding a review from a specific user or group, or adjusting the base branch.

An API endpoint exists at `api/simulate/:org/:repo/:prNumber` to simiulate the result of a pull request. Simulations using this endpoint will NOT write the result back to the pull request status check and will instead return the result.

This API requires a GitHub token be passed as a bearer token. The token must have the ability to read the pull request the simulation is being run against.

The API can be used as such:

```sh
$ curl https://policybot.domain/api/simulate/:org/:repo/:number -H 'authorization: Bearer <token>' -H 'content-type: application/json' -X POST -d '<data>'
```

Currently the data payload can be configured with a few options:

Ignore any comments from specific users, team members, org members or with specific permissions
```json
{
  "ignore_comments":{
    "users":["ignored-user"],
    "teams":["ignored-team"],
    "organizations":["ignored-org"],
    "permissions":["admin"]
  }
}
```

Ignore any reviews from specific users, team members, org members or with specific permissions
```json
{
  "ignore_reviews":{
    "users":["ignored-user"],
    "teams":["ignored-team"],
    "organizations":["ignored-org"],
    "permissions":["admin"]
  }
}
```

Simulate the pull request as if the following comments from the following users had also been added
```json
{
  "add_comments":[
    {
      "author":"not-ignored-user",
      "body":":+1:",
      "created_at": "2020-11-30T14:20:28.000+07:00",
      "last_edited_at": "2020-11-30T14:20:28.000+07:00"
    }
  ]
}
```

Simulate the pull request as if the following reviews from the following users had also been added
```json
{
  "add_reviews":[
    {
      "author":"not-ignored-user",
      "state": "approved",
      "body": "test approved review",
      "created_at": "2020-11-30T14:20:28.000+07:00",
      "last_edited_at": "2020-11-30T14:20:28.000+07:00"
    }
  ]
}
```

Choose a different base branch when simulating the pull request evaluation
```json
{
  "base_branch": "test-branch"
}
```

The above can be combined to form more complex simulations. If a Simulation is run without any data being passed, the pull request is evaluated as is.

### Caveats and Notes

There are several additional behaviors that follow from the rules above that
are worth mentioning.

#### Disapproval is Disabled by Default

You must set at least one of the `disapproval.requires` fields to enable
disapproval. Without setting one of these fields, GitHub reviews that request
changes have no effect on the `policy-bot` status.

#### Interactions with GitHub Reviews

GitHub Reviews allow a user to dismiss the last review they left, causing it to
no longer count towards rule evaluations. When this happens `policy-bot` will
use a previous, non-dismissed review, if it exists, when evaluating rules.

For example, if a user leaves an "approval" review and follows up with a
"request changes" review, `policy-bot` will use the "request changes" review
when evaluating rules. However, if the user then dimisses their "request
changes" review, `policy-bot` will instead use the initial "approval" review in
evaluating any rules.

#### `or`, `and`, and `if` (Rule Predicates)

If the `if` block of a rule (the predicate) is not satisfied, the rule is
marked as "skipped". Skipped rules interact with `or` and `and` as follows:

- An `and` block containing only skipped rules is also skipped
- An `or` block containing only skipped rules is also skipped

Effectively, skipped rules are treated as if they don't exist.

#### Cross-organization Membership Tests

`policy-bot` allows approval rules to reference organizations and teams that are
not in the organization that owns the repository where the rules appear. In
this case, `policy-bot` must be installed on all referenced organizations.

#### Update Merges

For a commit on a branch to count as an "update merge" for the purpose of the
`ignore_update_merges` option, the following must be true:

1. The commit must have exactly two parents
2. The commit must have the `committedViaWeb` property set to `true`
3. The first parent must exist in the pull request while the second parent
   must not exist in the pull request (meaning it is on the target branch)

These will all be true after updating a branch using the UI, but historic
merges on long-running branches or merges created with the API may not be
ignored. If this happens, you will need to reapprove the pull request.

This feature has [security implications](#update-merge-conflicts).

#### Automatically Requesting Reviewers

`policy-bot` can automatically request reviewers for all pending rules
when Pull Requests are opened by setting the `request_review` option.

The `mode` enum modifies how reviewers are selected. There are currently three
supported options:

 * `all-users` to request all users who can approve
 * `random-users` to randomly select the number of users that are required
 * `teams` to request teams for review. Teams must be repository collaborators
   with at least read access.

```yaml
options:
  request_review:
    enabled: true
    mode: all-users|random-users|teams
```

The set of requested reviewers will not include the author of the pull request or
users who are not collaborators on the repository.

When requesting reviews for rules that use repository permissions to select
approvers, only users who are direct collaborators or members of
repository teams are eligible for review selection. The users or their teams
must be granted an exact permission specified in the `permissions` list of the
rule.

For example, if a rule can be approved by any user with `admin` permission,
only direct or team admins are selected for review. Users who inherit
repository `admin` permissions as organization owners are not selected.

The `teams` mode needs the team visibility to be set to `visibile` to enable this functionality for a given team.

##### Example <!-- omit in toc -->

Given the following example requirement rule,

```yaml
  requires:
    count: 2
    users: ["user1", "user2"]
    organizations: ["org1", "org2"]
    teams: ["org1/team1", "org2/team2"]
```

`policy-bot` will attempt to request 2 reviewers randomly from the expanded
set of users of in

```yaml
["user1", "user2", "users in org1", "users in org2", "users in org1/team1", "users in org2/team"]
```

Where the Pull Request Author and any non direct collaborators have been removed
from the set.

#### Invalidating Approval on Push <!-- omit in toc -->

By default, `policy-bot` does not invalidate exisitng approvals when users add
new commits to a pull request. You can control this behavior for each rule in a
policy using the `invalidate_on_push` option.

To invalidate approvals, `policy-bot` compares an estimate of the push time of
each commit with the time of each approval comment or review. The push time
estimate uses the time of the oldest status check, or the current time during
evaluation if there are no status checks. This is guaranteed to be after the
actual push time, but the delay may be arbitrarily large based on GitHub
webhook delivery behavior and processing time in `policy-bot`.

In practice, this means that adding an approval immediately after (within a few
seconds of) a push may not approve the pull request. If this happens, leave a
second approval comment or review after `policy-bot` adds the "pending" status
check.

`policy-bot` caches push times in memory to improve performance and reduce API
requests.

Older versions of `policy-bot` (before 1.31.0) used the `pushedDate` field in
GitHub's GraphQL API to estimate commit push times. GitHub removed this field
in mid-2023 because computing it was unreliable and inaccurate (see issue
[#598][] for more details.)

[#598]: https://github.com/palantir/policy-bot/issues/598

#### Expanding Required Reviewers <!-- omit in toc -->

The details view for a pull request shows the users, organizations, teams, and
permission levels that are reqired to approve each rule. When the
`options.expand_required_reviewers` server option is set, `policy-bot` expands
these to show the list of users whose approval will satify each rule. This can
make it easier for developers to figure out who they should ask for approval.

Like with review requests, when expanding permission levels only users with
collaborator permissions on the repository, either directly or via teams, are
included in the expanded list.

Enabling this option can expose otherwise private information about teams,
organizations, and permissions to any user with read permission on a pull
request. This includes teams in organizations other than the one that contains
the pull request.

As a result, only enable this feature if all users with access to `policy-bot`
are allowed to view the members and permissions of any organization that uses
`policy-bot`.

## Security

While `policy-bot` can be used to implement security controls on GitHub
repositories, there are important limitations to be aware of before adopting
this approach.

### Status Checks <!-- omit in toc -->

`policy-bot` reports approval status to GitHub using [commit statuses][]. While
statuses cannot be deleted, they can be set or overwritten by any user with
write access to a repository. To prevent forged statuses, GitHub allows setting
an expected source for a status check when making it a [requirement on a
protected branch][]. Policy Bot always should be set as the expect source for
its checks.

For older versions of GitHub Enterprise that do not support expected sources
for status checks, `policy-bot` contains an auditing feature to detect
overwritten statuses. In addition to logging an audit event, it will replace
the forged status with a failure. However, a well-timed attempt can still
approve and merge a pull request before `policy-bot` can detect the problem.
Organizations concerned about this case should monitor and alert on the
relevant audit logs or minimize write access to repositories.

### Comment Edits <!-- omit in toc -->

GitHub users with sufficient permissions can edit the comments of other users,
possibly changing an unrelated comment into one that enables approval.
`policy-bot` also contains audting for this event, but as with statuses, a
well-timed edit can approve and merge a pull request before `policy-bot` can
detect the problem. Organizations concerned about this case can use the
`ignore_edited_comments` option or can monitor and alert on the relevant audit
logs.

This issue can also be minimized by only using GitHub reviews for approval, at
the expense of removing the ability to self-approve pull requests.

### Commit Users <!-- omit in toc -->

GitHub associates commits with users by mapping the email address in a commit
to email addresses associated with GitHub user accounts. `policy-bot` then uses
the GitHub username to evaluate user-based rules and options. There are two
failure modes in this process:

1. If GitHub does not recognize either the author or committer email of a
   commit, `policy-bot` cannot evaluate the commit with respect to user-based
   rules and the commit is effectively ignored.

2. If emails are manipulated when creating a commit, a user can trick GitHub
   and `policy-bot` into attributing the commit to a different user.

If using GitHub Enterprise, both of these issues are avoidable by using the
[commit-current-user-check][] pre-receive hook.

### Update Merge Conflicts <!-- omit in toc -->

When using the `ignore_update_merges` option, `policy-bot` cannot tell the
difference between clean merges and merges that contain conflict resolution.
This means that a user who carefully crafts a pull request to generate a
conflict can use the web conflict editor to add unapproved changes to the file
containing the conflict.

Depending on the author of the merge commits, it may be possible to avoid this
issue by using the `ignore_commits_by` option in combination with the
[commit-current-user-check][] pre-receive hook.

[commit statuses]: https://developer.github.com/v3/repos/statuses/
[requirement on a protected branch]: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/defining-the-mergeability-of-pull-requests/about-protected-branches#require-status-checks-before-merging
[commit-current-user-check]: https://github.com/github/platform-samples/blob/master/pre-receive-hooks/commit-current-user-check.sh

## Deployment

`policy-bot` is easy to deploy in your own environment as it has no dependencies
other than GitHub. It is also safe to run multiple instances of the server,
making it a good fit for container schedulers like Nomad or Kubernetes.

We provide both a Docker container and a binary distribution of the server:

- Binaries: https://github.com/palantir/policy-bot/releases
- Docker Images: https://hub.docker.com/r/palantirtechnologies/policy-bot/

A sample configuration file is provided at `config/policy-bot.example.yml`.
Certain values may also be set by environment variables; these are noted in the
comments in the sample configuration file. By default, the environment
variables for server values are prefixed with `POLICYBOT_` (e.g.
`POLICYBOT_PORT`). This prefix can be overridden by setting the
`POLICYBOT_ENV_PREFIX` environment variable.

### GitHub App Configuration <!-- omit in toc -->

To configure `policy-bot` as a GitHub App, set these options in GitHub:

- Under **Identifying and authorizing users**
  - Set **User authorization callback URL** to `http(s)://<your-policy-bot-domain>/api/github/auth`
  - Uncheck **Request user authorization (OAuth) during installation**
- Under **Webhook**
  - Set **Webhook URL** to `http(s)://<your-policy-bot-domain>/api/github/hook`
  - Set **Webhook secret**: A random string that matches the value of the
    `github.app.webhook_secret` property in the server configuration

The app requires these permissions:

| Permission | Access | Reason |
| ---------- | ------ | ------ |
| Repository contents | Read-only | Read configuration and commit metadata |
| Checks | Read-only | Read check run results |
| Repository administration | Read-only | Read admin team(s) membership |
| Issues | Read-only | Read pull request comments |
| Merge Queues | Read-only | Read repository merge queues |
| Repository metadata | Read-only | Basic repository data |
| Pull requests | Read & write | Receive pull request events, read metadata. Assign reviewers |
| Commit status | Read & write | Post commit statuses |
| Organization members | Read-only | Determine organization and team membership |

The app should be subscribed to these events:

* Check run
* Issue comment
* Merge groups
* Pull request
* Pull request review
* Status

There is a [`logo.png`](https://github.com/palantir/policy-bot/blob/develop/logo.png)
provided if you'd like to use it as the GitHub application logo. The background
color is `#4d4d4d`.

After creating the app, update the server configuration file with the following
generated values:

- App ID (`github.app.integration_id`)
- Client ID (`github.oauth.client_id`)
- Client secret (`github.oauth.client_secret`)
- Private key (`github.app.private_key`)

### Operations <!-- omit in toc -->

`policy-bot` uses [go-baseapp](https://github.com/palantir/go-baseapp) and
[go-githubapp](https://github.com/palantir/go-githubapp), both of which emit
standard metrics and structured log keys. Please see those projects for
details.

## Development

To develop `policy-bot`, you will need a [Go installation](https://golang.org/doc/install).
If you want to build the UI, you'll also need [NodeJS](https://nodejs.org/en/)
and [Yarn](https://yarnpkg.com/).

**Run style checks and tests**

    ./godelw verify

**Running the server locally**

    # copy and edit the server config
    cp config/policy-bot.example.yml config/policy-bot.yml

    ./godelw run policy-bot server

- `config/policy-bot.yml` is used as the default configuration file
- The server is available at `http://localhost:8080/`

**Installing UI dependencies and building assets**

    # install dependencies
    yarn install

    # build CSS and JS assets
    yarn run build

- This generates a combined stylesheet with `policy-bot` styles and
  [Tailwind](https://tailwindcss.com/) core styles. It also copies JS files and
  other assets into the correct locations.
- To use the local asset files with a local server, add or uncomment the
  following in the server configuration file:

  ```yaml
  files:
    static: build/static
    templates: server/templates
  ```

**Running the server via docker**

    # copy and edit the server config
    cp config/policy-bot.example.yml config/policy-bot.yml

    # build the docker image
    ./godelw docker build --verbose

    docker run --rm -v "$(pwd)/config:/secrets/" -p 8080:8080 palantirtechnologies/policy-bot:latest

- This will mount the path relative path `config/` which should contain the
  modified config file `policy-bot.yml`
- The server is available at `http://localhost:8080/`

### Example Policy Files <!-- omit in toc -->

Example policy files can be found in [`config/policy-examples`](https://github.com/palantir/policy-bot/tree/develop/config/policy-examples)

## Contributing

Contributions and issues are welcome. For new features or large contributions,
we prefer discussing the proposed change on a GitHub issue prior to a PR.

## License

This library is made available under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0).
