package fail

fail

/*
This is a non-compiling file that has been added to explicitly ensure that CI fails.
It also contains the command that caused the failure and its output.
Remove this file if debugging locally.

go mod operation failed. This may mean that there are legitimate dependency issues with the "go.mod" definition in the repository and the updates performed by the gomod check. This branch can be cloned locally to debug the issue.

Command that caused error:
./godelw lint compiles

Output:
server/handler/audit.go:1: : # github.com/palantir/policy-bot/server/handler
server/handler/base.go:53:65: cannot use pr.GetBase().GetRepo() (value of type *"github.com/google/go-github/v74/github".Repository) as *"github.com/google/go-github/v75/github".Repository value in argument to githubapp.PreparePRContext
server/handler/base.go:72:46: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in argument to NewCrossOrgMembershipContext
server/handler/base.go:73:66: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in argument to pull.NewGitHubContext
server/handler/base.go:82:66: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in argument to b.ConfigFetcher.ConfigForRepositoryBranch
server/handler/base.go:85:13: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in struct literal
server/handler/check_run.go:49:57: cannot use &event (value of type *"github.com/google/go-github/v74/github".CheckRunEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v74/github".CheckRunEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v74/github".Installation
		want GetInstallation() *"github.com/google/go-github/v75/github".Installation
server/handler/check_run.go:51:67: cannot use repo (variable of type *"github.com/google/go-github/v74/github".Repository) as *"github.com/google/go-github/v75/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/cross_org.go:66:51: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in argument to pull.NewGitHubMembershipContext
server/handler/details.go:144:66: cannot use client (variable of type *"github.com/google/go-github/v75/github".Client) as *"github.com/google/go-github/v74/github".Client value in argument to checkUserPermissions
server/handler/details.go:164:58: cannot use pr (variable of type *"github.com/google/go-github/v75/github".PullRequest) as *"github.com/google/go-github/v74/github".PullRequest value in argument to h.PreparePRContext
server/handler/details.go:164:58: too many errors (typecheck)
// Copyright 2018 Palantir Technologies, Inc.
1 issues:
* typecheck: 1

*/
