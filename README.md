# policy-bot

[![Download](https://api.bintray.com/packages/palantir/releases/policy-bot/images/download.svg)](https://bintray.com/palantir/releases/policy-bot/_latestVersion) [![Docker Pulls](https://img.shields.io/docker/pulls/palantirtechnologies/policy-bot.svg)](https://hub.docker.com/r/palantirtechnologies/policy-bot/)

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

* [Configuration](#configuration)
  + [policy.yml Specification](#policyyml-specification)
  + [Approval Rules](#approval-rules)
  + [Approval Policies](#approval-policies)
  + [Disapproval](#disapproval)
  + [Caveats and Notes](#caveats-and-notes)
    - [Disapproval is Disabled by Default](#disapproval-is-disabled-by-default)
    - [`or`, `and`, and `if` (Rule Predicates)](#or-and-and-if-rule-predicates)
    - [Cross-organization Membership Tests](#cross-organization-membership-tests)
    - [Update Merges](#update-merges)
* [Deployment](#deployment)
* [Development](#development)
* [Contributing](#contributing)
* [License](#license)

## Configuration

By default, the behavior of the bot is configured by a `.policy.yml` file at
the root of the repository. The file name and location are configurable when
running your own instance of the server.

- If the file does not exist, the `policy-bot` status check is not posted. This
  means it is safe to enable `policy-bot` on all repositories in an organization.
- The `.policy.yml` file is read from the most recent commit on the target branch
  of each pull request.

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
          - "staging/.*"
    requires:
      count: 0
```

#### Remote Policy Configuration
You can also define a remote policy by specifying a repository, path, and ref
(only repository is required). Instead of defining a `policy` key, you would
define a `remote` key. Only 1 level of remote configuration is supported by design.

```yaml
# The remote repository to read the policy file from. This is required, and must
# be in the form of "org/repo-name". Must be a public repository.
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

# "if" specifies a set of predicates that must be true for the rule to apply.
# This block, and every condition within it are optional. If the block does not
# exist, the rule applies to every pull request.
if:
  # "changed_files" is satisfied if any file in the pull request matches any
  # regular expression in the list.
  changed_files:
    paths:
      - "config/.*"
      - "server/views/.*\\.tmpl"

  # "only_changed_files" is satisfied if all files changed by the pull request
  # match at least one regular expression in the list.
  only_changed_files:
    paths:
      - "config/.*"

  # "has_author_in" is satisified if the user who opened the pull request is in
  # the users list or belongs to any of the listed organizations or teams.
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

  # "targets_branch" is satisfied if the target branch on the pull request
  # matches the regular expression
  targets_branch:
    pattern: "^(master|regexPattern)$"

# "options" specifies a set of restrictions on approvals. If the block does not
# exist, the default values are used.
options:
  # If true, approvals by the author of a pull request are considered when
  # calculating the status. False by default.
  allow_author: false

  # If true, the approvals of someone who has committed to the pull request are
  # considered when calculating the status. False by default.
  allow_contributor: false

  # If true, pushing new commits to a pull request will invalidate existing
  # approvals for this rule. False by default.
  invalidate_on_push: false

  # If true, "update merges" do not invalidate approval (if invalidate_on_push
  # is enabled) and their authors/committers do not count as contributors. An
  # "update merge" is a merge commit that was created in the UI or via the API
  # and merges the target branch into the pull request branch. These are
  # commonly created by using the "Update branch" button in the UI.
  ignore_update_merges: false

  # "methods" defines how users may express approval. The defaults are below.
  methods:
    comments:
      - ":+1:"
      - "👍"
    github_review: true

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

  # "github" resolves the membership with respect to github settings for the repo
  # using the API for both organization and repository settings
  github:
    # allows approval by admins of the org or repository
    admin: true
    # allows approval by users who have write on the repository
    write_collaborator: true
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

### Disapproval

Disapproval allows users to explicitly block pull requests if certain changes
must be made. Any member of in the set of allowed users can disapprove a change
or revoke another user's disapproval.

Unlike approval, all disapproval options are specified as part of the policy.
Effectively, there is a single disapproval rule. The `disapproval` policy has
the following specification:

```yaml
# "disapproval" is the top-level key in the policy block.
disapproval:
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

### Caveats and Notes

There are several additional behaviors that follow from the rules above that
are worth mentioning.

#### Disapproval is Disabled by Default

You must set at least one of the `disapproval.requires` fields to enable
disapproval. Without setting one of these fields, GitHub reviews that request
changes have no effect on the `policy-bot` status.

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
3. One parent must exist in the last 100 commits on the target branch of the
   pull request

These will all be true after updating a branch using the UI, but historic
merges on long-running branches or merges created with the API may not be
ignored. If this happens, you will need to reapprove the pull request.

Note that `policy-bot` cannot detect if an update merge contains any merge
conflict resolutions. If you enable this option, users _may_ be able to merge
unapproved code by exploiting the conflict editor.

## Deployment

`policy-bot` is easy to deploy in your own environment as it has no dependencies
other than GitHub. It is also safe to run multiple instances of the server,
making it a good fit for container schedulers like Nomad or Kubernetes.

We provide both a Docker container and a binary distribution of the server:

- Binaries: https://bintray.com/palantir/releases/policy-bot
- Docker Images: https://hub.docker.com/r/palantirtechnologies/policy-bot/

A sample configuration file is provided at `config/policy-bot.example.yml`. We
recommend deploying the application behind a reverse proxy or load balancer
that terminates TLS connections.

### GitHub App Configuration

`policy-bot` requires the following permissions as a GitHub app:

| Permission | Access | Reason |
| ---------- | ------ | ------ |
| Repository contents | Read & write | Read configuration, perform merges |
| Issues | Read-only | Read pull request comments |
| Repository metadata | Read-only | Basic repository data |
| Pull requests | Read-only| Receive pull request events, read metadata |
| Commit status | Read & write | Post commit statuses |
| Organization members | Read-only | Determine organization and team membership |

It should be subscribed to the following events:

* Issue comment
* Pull request
* Status
* Pull request review

There is a [`logo.png`](https://github.com/palantir/policy-bot/blob/develop/logo.png)
provided if you'd like to use it as the GitHub application logo. The background
color is `#4d4d4d`.

### Operations

`policy-bot` uses [go-baseapp](https://github.com/palantir/go-baseapp) and
[go-githubapp](https://github.com/palantir/go-githubapp), both of which emit
standard metrics and structured log keys. Please see those projects for
details.

## Development

To develop `policy-bot`, you will need a [Go installation](https://golang.org/doc/install).

**Run style checks and tests**

    ./godelw verify

**Running the server locally**

    # copy and edit the server config
    cp config/policy-bot.example.yml config/policy-bot.yml

    ./godelw run policy-bot server

- `config/policy-bot.yml` is used as the default configuration file
- The server is available at `http://localhost:8080/`

**Running the server via docker**

    # copy and edit the server config
    cp config/policy-bot.example.yml config/policy-bot.yml
    
    # build the docker image
    ./godelw docker build --verbose

    docker run --rm -v "$(pwd)/config:/secrets/" -p 8080:8080 palantirtechnologies/policy-bot:latest

- This will mount the path relative path `config/` which should contain the
  modified config file `policy-bot.yml`
- The server is available at `http://localhost:8080/`

### Example Policy Files
Example policy files can be found in [`config/policy-examples`](https://github.com/palantir/policy-bot/tree/develop/config/policy-examples)

## Contributing

Contributions and issues are welcome. For new features or large contributions,
we prefer discussing the proposed change on a GitHub issue prior to a PR.

## License

This library is made available under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0).
