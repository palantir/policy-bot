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
	"net/http"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// Installation is a minimal representation of
// an installation ID and it's corresponding
// owner.
type Installation struct {
	ID      int64
	Owner   string
	OwnerID int64
}

// InstallationClient is used to retrieve the installation ID from Github
// for a given App. It is an implementation detail how results are
// sourced, stored, and cached (or not).
// This is useful for apps that have background processes
// that do not responding directly to webhooks, since the webhooks
// otherwise provide the installation ID in their payloads.
type InstallationClient interface {
	ListAll(ctx context.Context) ([]Installation, error)
	GetByOwner(ctx context.Context, owner string) (Installation, error)
}

type DefaultInstallationClient struct {
	appClient *github.Client
}

func NewInstallationClient(appClient *github.Client) *DefaultInstallationClient {
	return &DefaultInstallationClient{
		appClient: appClient,
	}
}

func toInstallation(from *github.Installation) Installation {
	return Installation{
		ID:      from.GetID(),
		Owner:   from.GetAccount().GetLogin(),
		OwnerID: from.GetAccount().GetID(),
	}
}

func isNotFound(err error) bool {
	rerr, ok := err.(*github.ErrorResponse)
	return ok && rerr.Response.StatusCode == http.StatusNotFound
}

func (i *DefaultInstallationClient) ListAll(ctx context.Context) ([]Installation, error) {
	opt := github.ListOptions{
		PerPage: 100,
	}

	var allInstallations []Installation
	for {
		installations, res, err := i.appClient.Apps.ListInstallations(ctx, &opt)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list installations")
		}
		for _, inst := range installations {
			allInstallations = append(allInstallations, toInstallation(inst))
		}
		if res.NextPage == 0 {
			break
		}
		opt.Page = res.NextPage
	}

	return allInstallations, nil
}

func (i *DefaultInstallationClient) GetByOwner(ctx context.Context, owner string) (Installation, error) {
	installation, _, err := i.appClient.Apps.FindOrganizationInstallation(ctx, owner)
	if err != nil {
		if isNotFound(err) {
			return Installation{}, errors.Errorf("no installation found for owner %q", owner)
		}
		return Installation{}, errors.Wrapf(err, "failed to list installation for owner %q", owner)
	}

	return toInstallation(installation), nil
}

// type assertion
var _ InstallationClient = &DefaultInstallationClient{}
