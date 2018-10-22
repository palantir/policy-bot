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
	"fmt"
	"net/http"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// Installation is a minimal representation of a GitHub app installation.
type Installation struct {
	ID      int64
	Owner   string
	OwnerID int64
}

// InstallationSource is implemented by GitHub webhook event payload types.
type InstallationSource interface {
	GetInstallation() *github.Installation
}

// GetInstallationIDFromEvent returns the installation ID from a GitHub webhook
// event payload.
func GetInstallationIDFromEvent(event InstallationSource) int64 {
	return event.GetInstallation().GetID()
}

// InstallationsService retrieves installation information for a given app.
// Implementations may chose how to retrieve, store, or cache these values.
//
// This service is useful for background processes that do not respond directly
// to webhooks, since webhooks provide installation IDs in their payloads.
type InstallationsService interface {
	// ListAll returns all installations for this app.
	ListAll(ctx context.Context) ([]Installation, error)

	// GetByOwner returns the installation for an owner (user or organization).
	// It returns an InstallationNotFound error if no installation exists for
	// the owner.
	GetByOwner(ctx context.Context, owner string) (Installation, error)
}

type defaultInstallationsService struct {
	*github.Client
}

// NewInstallationsService returns an InstallationsService that always queries
// GitHub. It should be created with a client that authenticates as the target
// application.
func NewInstallationsService(appClient *github.Client) InstallationsService {
	return defaultInstallationsService{appClient}
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

func (i defaultInstallationsService) ListAll(ctx context.Context) ([]Installation, error) {
	opt := github.ListOptions{
		PerPage: 100,
	}

	var allInstallations []Installation
	for {
		installations, res, err := i.Apps.ListInstallations(ctx, &opt)
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

func (i defaultInstallationsService) GetByOwner(ctx context.Context, owner string) (Installation, error) {
	installation, _, err := i.Apps.FindOrganizationInstallation(ctx, owner)
	if err == nil {
		return toInstallation(installation), nil
	}

	// owner is not an organization, try to find as a user
	if isNotFound(err) {
		installation, _, err = i.Apps.FindUserInstallation(ctx, owner)
		if err == nil {
			return toInstallation(installation), nil
		}
	}

	if isNotFound(err) {
		return Installation{}, InstallationNotFound(owner)
	}
	return Installation{}, errors.Wrapf(err, "failed to get installation for owner %q", owner)
}

// InstallationNotFound is returned when no installation exists for a
// specific owner or repository.
type InstallationNotFound string

func (err InstallationNotFound) Error() string {
	return fmt.Sprintf("no installation found for %q", string(err))
}
