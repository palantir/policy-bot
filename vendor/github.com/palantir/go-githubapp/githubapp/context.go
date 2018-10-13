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

package githubapp

import (
	"context"

	"github.com/google/go-github/github"
	"github.com/rs/zerolog"
)

const (
	LogKeyEventType       string = "github_event_type"
	LogKeyDeliveryID      string = "github_delivery_id"
	LogKeyRepositoryName  string = "github_repository_name"
	LogKeyRepositoryOwner string = "github_repository_owner"
	LogKeyPRNum           string = "github_pr_num"
	LogKeyInstallationID  string = "github_installation_id"
)

// PrepareRepoContext adds information about a repository to the logger in a
// context and returns the modified context and logger.
func PrepareRepoContext(ctx context.Context, installationID int64, repo *github.Repository) (context.Context, zerolog.Logger) {
	logctx := zerolog.Ctx(ctx).With()

	logctx = attachInstallationLogKeys(logctx, installationID)
	logctx = attachRepoLogKeys(logctx, repo)

	logger := logctx.Logger()
	return logger.WithContext(ctx), logger
}

// PreparePRContext adds information about a pull request to the logger in a
// context and returns the modified context and logger.
func PreparePRContext(ctx context.Context, installationID int64, repo *github.Repository, number int) (context.Context, zerolog.Logger) {
	logctx := zerolog.Ctx(ctx).With()

	logctx = attachInstallationLogKeys(logctx, installationID)
	logctx = attachRepoLogKeys(logctx, repo)
	logctx = attachPullRequestLogKeys(logctx, number)

	logger := logctx.Logger()
	return logger.WithContext(ctx), logger
}

func attachInstallationLogKeys(logctx zerolog.Context, installID int64) zerolog.Context {
	if installID > 0 {
		return logctx.Int64(LogKeyInstallationID, installID)
	}
	return logctx
}

func attachRepoLogKeys(logctx zerolog.Context, repo *github.Repository) zerolog.Context {
	if repo != nil {
		return logctx.
			Str(LogKeyRepositoryOwner, repo.GetOwner().GetLogin()).
			Str(LogKeyRepositoryName, repo.GetName())
	}
	return logctx
}

func attachPullRequestLogKeys(logctx zerolog.Context, number int) zerolog.Context {
	if number > 0 {
		return logctx.Int(LogKeyPRNum, number)
	}
	return logctx
}
