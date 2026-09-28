package fail

fail

/*
This is a non-compiling file that has been added to explicitly ensure that CI fails.
It also contains the command that caused the failure and its output.
Remove this file if debugging locally.

go mod operation failed. This may mean that there are legitimate dependency issues with the "go.mod" definition in the repository and the updates performed by the bump-go-dependencies check. This branch can be cloned locally to debug the issue.

Command that caused error:
./godelw lint --fix

Output:
-: # github.com/palantir/policy-bot/server/handler
server/handler/base.go:53:65: cannot use pr.GetBase().GetRepo() (value of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PreparePRContext
server/handler/base.go:72:46: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to NewCrossOrgMembershipContext
server/handler/base.go:73:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubContext
server/handler/base.go:82:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to b.ConfigFetcher.ConfigForRepositoryBranch
server/handler/base.go:85:13: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in struct literal
server/handler/check_run.go:49:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".CheckRunEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".CheckRunEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/check_run.go:51:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/cross_org.go:66:51: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubMembershipContext
server/handler/details.go:144:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to checkUserPermissions
server/handler/details.go:164:58: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in argument to h.PreparePRContext
server/handler/details.go:164:58: too many errors
server/handler/base.go:53:65: cannot use pr.GetBase().GetRepo() (value of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PreparePRContext
server/handler/base.go:72:46: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to NewCrossOrgMembershipContext
server/handler/base.go:73:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubContext
server/handler/base.go:82:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to b.ConfigFetcher.ConfigForRepositoryBranch
server/handler/base.go:85:13: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in struct literal
server/handler/check_run.go:49:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".CheckRunEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".CheckRunEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/check_run.go:51:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/cross_org.go:66:51: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubMembershipContext
server/handler/details.go:144:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to checkUserPermissions
server/handler/details.go:164:58: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in argument to h.PreparePRContext
server/handler/details.go:169:11: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in struct literal
server/handler/details.go:180:16: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in struct literal
server/handler/installation.go:53:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".InstallationEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".InstallationEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/installation.go:63:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".InstallationRepositoriesEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".InstallationRepositoriesEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/installation.go:74:38: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to h.postRepoInstallationStatus
server/handler/issue_comment.go:48:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".IssueCommentEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".IssueCommentEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/issue_comment.go:65:57: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in argument to h.PreparePRContext
server/handler/issue_comment.go:71:11: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in struct literal
server/handler/merge_group.go:49:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".MergeGroupEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".MergeGroupEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/merge_group.go:63:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to h.ConfigFetcher.ConfigForRepositoryBranch
server/handler/merge_group.go:77:28: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to PostStatus
server/handler/merge_group.go:83:29: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to PostStatus
server/handler/pull_request.go:42:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".PullRequestEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".PullRequestEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/pull_request_review.go:52:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".PullRequestReviewEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".PullRequestReviewEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/simulate.go:88:52: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to getUserSimulatePermission
server/handler/simulate.go:101:52: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in argument to h.PreparePRContext
server/handler/simulate.go:111:11: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in struct literal
server/handler/simulate.go:162:46: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to NewCrossOrgMembershipContext
server/handler/simulate.go:163:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubContext
server/handler/simulate.go:172:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to h.ConfigFetcher.ConfigForRepositoryBranch
server/handler/status.go:61:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".StatusEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".StatusEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/status.go:68:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/status.go:99:84: cannot use status (variable of struct type "github.com/google/go-github/v90/github".RepoStatus) as "github.com/google/go-github/v92/github".RepoStatus value in argument to client.Repositories.CreateStatus
server/handler/status.go:108:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".StatusEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".StatusEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/status.go:115:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/status.go:124:3: cannot use &github.ListOptions{…} (value of type *"github.com/google/go-github/v90/github".ListOptions) as *"github.com/google/go-github/v92/github".ListOptions value in argument to client.PullRequests.ListPullRequestsWithCommit
server/handler/status.go:139:13: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in struct literal
server/handler/workflow_run.go:51:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".WorkflowRunEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".WorkflowRunEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/workflow_run.go:53:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/server.go:172:12: cannot use appconfig.NewLoader(policyPaths, appconfig.WithOwnerDefault(*c.Options.SharedRepository, sharedPolicyPaths)) (value of type *appconfig.Loader) as handler.ConfigLoader value in struct literal: *appconfig.Loader does not implement handler.ConfigLoader (wrong type for method LoadConfig)
		have LoadConfig(context.Context, *"github.com/google/go-github/v92/github".Client, string, string, string) (appconfig.Config, error)
		want LoadConfig(context.Context, *"github.com/google/go-github/v90/github".Client, string, string, string) (appconfig.Config, error)
-: # github.com/palantir/policy-bot/server/handler [github.com/palantir/policy-bot/server/handler.test]
server/handler/base.go:53:65: cannot use pr.GetBase().GetRepo() (value of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PreparePRContext
server/handler/base.go:72:46: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to NewCrossOrgMembershipContext
server/handler/base.go:73:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubContext
server/handler/base.go:82:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to b.ConfigFetcher.ConfigForRepositoryBranch
server/handler/base.go:85:13: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in struct literal
server/handler/check_run.go:49:57: cannot use &event (value of type *"github.com/google/go-github/v90/github".CheckRunEvent) as githubapp.InstallationSource value in argument to githubapp.GetInstallationIDFromEvent: *"github.com/google/go-github/v90/github".CheckRunEvent does not implement githubapp.InstallationSource (wrong type for method GetInstallation)
		have GetInstallation() *"github.com/google/go-github/v90/github".Installation
		want GetInstallation() *"github.com/google/go-github/v92/github".Installation
server/handler/check_run.go:51:67: cannot use repo (variable of type *"github.com/google/go-github/v90/github".Repository) as *"github.com/google/go-github/v92/github".Repository value in argument to githubapp.PrepareRepoContext
server/handler/cross_org.go:66:51: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to pull.NewGitHubMembershipContext
server/handler/details.go:144:66: cannot use client (variable of type *"github.com/google/go-github/v92/github".Client) as *"github.com/google/go-github/v90/github".Client value in argument to checkUserPermissions
server/handler/details.go:164:58: cannot use pr (variable of type *"github.com/google/go-github/v92/github".PullRequest) as *"github.com/google/go-github/v90/github".PullRequest value in argument to h.PreparePRContext
server/handler/details.go:164:58: too many errors
42 issues:
* compiles: 42

*/
