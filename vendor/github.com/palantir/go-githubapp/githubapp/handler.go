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

type BaseHandler struct {
	ClientCreator ClientCreator
}

func NewDefaultBaseHandler(c Config, opts ...ClientOption) (BaseHandler, error) {
	delegate := NewClientCreator(
		c.V3APIURL,
		c.V4APIURL,
		c.App.IntegrationID,
		[]byte(c.App.PrivateKey),
		opts...,
	)

	cc, err := NewCachingClientCreator(delegate, DefaultCachingClientCapacity)
	if err != nil {
		return BaseHandler{}, err
	}

	return BaseHandler{
		ClientCreator: cc,
	}, nil
}

type InstallationSource interface {
	GetInstallation() *github.Installation
}

func (b *BaseHandler) GetInstallationIDFromEvent(event InstallationSource) int64 {
	return event.GetInstallation().GetID()
}

func (b *BaseHandler) PreparePRContext(ctx context.Context, installationID int64, repo *github.Repository, prNum int) (context.Context, *github.Client, error) {
	client, err := b.ClientCreator.NewInstallationClient(installationID)
	if err != nil {
		return nil, nil, err
	}

	logger := zerolog.Ctx(ctx).
		With().
		Str(LogKeyRepositoryOwner, repo.GetOwner().GetLogin()).
		Str(LogKeyRepositoryName, repo.GetName()).
		Int(LogKeyPRNum, prNum).
		Int64(LogKeyInstallationID, installationID).
		Logger()

	return logger.WithContext(ctx), client, nil
}

func (b *BaseHandler) PrepareRepoContext(ctx context.Context, installationID int64, repo *github.Repository) (context.Context, *github.Client, error) {
	client, err := b.ClientCreator.NewInstallationClient(installationID)
	if err != nil {
		return nil, nil, err
	}

	logger := zerolog.Ctx(ctx).
		With().
		Str(LogKeyRepositoryOwner, repo.GetOwner().GetLogin()).
		Str(LogKeyRepositoryName, repo.GetName()).
		Int64(LogKeyInstallationID, installationID).
		Logger()

	return logger.WithContext(ctx), client, nil
}

func (b *BaseHandler) PrepareOrgContext(ctx context.Context, installationID int64, org *github.Organization) (context.Context, *github.Client, error) {
	client, err := b.ClientCreator.NewInstallationClient(installationID)
	if err != nil {
		return nil, nil, err
	}

	logger := zerolog.Ctx(ctx).
		With().
		Str(LogKeyRepositoryOwner, org.GetLogin()).
		Int64(LogKeyInstallationID, installationID).
		Logger()

	return logger.WithContext(ctx), client, nil
}
