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

package pull

import (
	"context"
	"strings"
	"sync"

	"github.com/google/go-github/v81/github"
	"github.com/pkg/errors"
)

type GitHubMembershipContext struct {
	ctx         context.Context
	client      *github.Client
	globalCache GlobalCache

	membership  sync.Map // map[string]bool
	orgMembers  sync.Map // map[string][]string
	teamMembers sync.Map // map[string][]string
	teamData    sync.Map // map[string]*teamLoader
}

// cachedTeamData holds rich team data including member details
type cachedTeamData struct {
	Info    *TeamInfo
	Members []TeamMember
}

// teamLoader coordinates loading of team data to prevent duplicate API calls
type teamLoader struct {
	once sync.Once
	data *cachedTeamData
	err  error
}

func NewGitHubMembershipContext(ctx context.Context, client *github.Client, globalCache GlobalCache) *GitHubMembershipContext {
	return &GitHubMembershipContext{
		ctx:         ctx,
		client:      client,
		globalCache: globalCache,
		// sync.Map zero value is ready to use
	}
}

func membershipKey(group, user string) string {
	return group + ":" + user
}

func splitTeam(team string) (org, slug string, err error) {
	parts := strings.Split(team, "/")
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid team format: %s", team)
	}
	return parts[0], parts[1], nil
}

func (mc *GitHubMembershipContext) IsTeamMember(team, user string) (bool, error) {
	key := membershipKey(team, user)

	// Check per-request cache first (fast path)
	if val, ok := mc.membership.Load(key); ok {
		return val.(bool), nil
	}

	// Check global caches
	if mc.globalCache != nil {
		if isMember, found := mc.checkGlobalCaches(key, team, user); found {
			return isMember, nil
		}
	}

	// Fetch full team members list (batch approach is more efficient for multiple checks)
	data, err := mc.loadTeamWithMembers(team)
	if err != nil {
		// Fall back to individual API call on error
		return mc.checkTeamMembershipViaAPI(key, team, user)
	}

	// populateLocalCaches already stored true for all members, but we need to
	// also store false for non-members to avoid re-checking the list
	isMember := mc.findMemberInList(data.Members, user)
	if !isMember {
		mc.membership.Store(key, false)
	}
	return isMember, nil
}

// checkGlobalCaches checks both team members cache and individual membership cache.
// Returns (isMember, found) where found indicates if a cached value was found.
func (mc *GitHubMembershipContext) checkGlobalCaches(key, team, user string) (bool, bool) {
	// Check full team members list first
	if members, _, found := mc.globalCache.GetTeamMembers(team); found {
		isMember := mc.findMemberInList(members, user)
		mc.membership.Store(key, isMember)
		return isMember, true
	}

	// Fall back to individual membership cache
	if isMember, found := mc.globalCache.GetTeamMembership(team, user); found {
		mc.membership.Store(key, isMember)
		return isMember, true
	}

	return false, false
}

// findMemberInList searches for a user in a team member list using case-insensitive comparison.
func (mc *GitHubMembershipContext) findMemberInList(members []TeamMember, user string) bool {
	for _, m := range members {
		if strings.EqualFold(m.Login, user) {
			return true
		}
	}
	return false
}

// checkTeamMembershipViaAPI checks team membership using individual API call.
// Used as fallback when batch fetch fails.
func (mc *GitHubMembershipContext) checkTeamMembershipViaAPI(key, team, user string) (bool, error) {
	org, slug, err := splitTeam(team)
	if err != nil {
		return false, err
	}

	membership, _, err := mc.client.Teams.GetTeamMembershipBySlug(mc.ctx, org, slug, user)
	if err != nil {
		if isNotFound(err) {
			mc.membership.Store(key, false)
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get team membership")
	}

	isMember := membership.GetState() == "active"
	mc.membership.Store(key, isMember)
	if mc.globalCache != nil {
		mc.globalCache.SetTeamMembership(team, user, isMember)
	}

	return isMember, nil
}

func (mc *GitHubMembershipContext) IsOrgMember(org, user string) (bool, error) {
	key := membershipKey(org, user)

	if val, ok := mc.membership.Load(key); ok {
		return val.(bool), nil
	}

	isMember, _, err := mc.client.Organizations.IsMember(mc.ctx, org, user)
	if err != nil {
		return false, errors.Wrap(err, "failed to get organization membership")
	}

	mc.membership.Store(key, isMember)
	return isMember, nil
}

func (mc *GitHubMembershipContext) OrganizationMembers(org string) ([]string, error) {
	if val, ok := mc.orgMembers.Load(org); ok {
		return val.([]string), nil
	}

	opt := &github.ListMembersOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var members []string
	for {
		users, resp, err := mc.client.Organizations.ListMembers(mc.ctx, org, opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list members of org %s page %d", org, opt.Page)
		}
		for _, u := range users {
			members = append(members, u.GetLogin())
			// And cache these values for later lookups
			mc.membership.Store(membershipKey(org, u.GetLogin()), true)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	mc.orgMembers.Store(org, members)
	return members, nil
}

func (mc *GitHubMembershipContext) TeamMembers(team string) ([]string, error) {
	if val, ok := mc.teamMembers.Load(team); ok {
		return val.([]string), nil
	}

	org, slug, err := splitTeam(team)
	if err != nil {
		return nil, err
	}

	opt := &github.TeamListTeamMembersOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var members []string
	for {
		users, resp, err := mc.client.Teams.ListTeamMembersBySlug(mc.ctx, org, slug, opt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list team %s members page %d", team, opt.Page)
		}
		for _, u := range users {
			login := u.GetLogin()
			members = append(members, login)
			mc.membership.Store(membershipKey(team, login), true)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	mc.teamMembers.Store(team, members)
	return members, nil
}

// loadTeamWithMembers fetches and caches team data with full member details.
// It checks global cache first, then uses sync.Once to deduplicate concurrent API calls.
func (mc *GitHubMembershipContext) loadTeamWithMembers(team string) (*cachedTeamData, error) {
	// Check global cache first (shared across requests)
	if mc.globalCache != nil {
		if members, info, found := mc.globalCache.GetTeamMembers(team); found {
			mc.populateLocalCaches(team, members)
			return &cachedTeamData{Info: info, Members: members}, nil
		}
	}

	// Use LoadOrStore to get-or-create a loader, ensuring only one fetch per team
	loaderVal, _ := mc.teamData.LoadOrStore(team, &teamLoader{})
	loader := loaderVal.(*teamLoader)

	// Execute the fetch exactly once
	loader.once.Do(func() {
		loader.data, loader.err = mc.fetchTeamFromAPI(team)
	})

	return loader.data, loader.err
}

// populateLocalCaches updates per-request caches from team member data.
func (mc *GitHubMembershipContext) populateLocalCaches(team string, members []TeamMember) {
	usernames := make([]string, len(members))
	for i, m := range members {
		mc.membership.Store(membershipKey(team, m.Login), true)
		usernames[i] = m.Login
	}
	mc.teamMembers.Store(team, usernames)
}

// fetchTeamFromAPI fetches team info and members from the GitHub API in parallel.
func (mc *GitHubMembershipContext) fetchTeamFromAPI(team string) (*cachedTeamData, error) {
	org, slug, err := splitTeam(team)
	if err != nil {
		return nil, err
	}

	var teamObj *github.Team
	var members []TeamMember
	var teamErr, membersErr error
	var wg sync.WaitGroup

	// Fetch team info
	wg.Go(func() {
		t, _, err := mc.client.Teams.GetTeamBySlug(mc.ctx, org, slug)
		if err != nil {
			teamErr = errors.Wrap(err, "failed to get team info")
			return
		}
		teamObj = t
	})

	// Fetch team members
	wg.Go(func() {
		opt := &github.TeamListTeamMembersOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			users, resp, err := mc.client.Teams.ListTeamMembersBySlug(mc.ctx, org, slug, opt)
			if err != nil {
				membersErr = errors.Wrapf(err, "failed to list team %s members page %d", team, opt.Page)
				return
			}
			for _, u := range users {
				members = append(members, TeamMember{
					Login:     u.GetLogin(),
					AvatarURL: u.GetAvatarURL(),
					HTMLURL:   u.GetHTMLURL(),
				})
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
	})

	wg.Wait()

	// Return first error encountered
	if teamErr != nil {
		return nil, teamErr
	}
	if membersErr != nil {
		return nil, membersErr
	}

	info := &TeamInfo{
		ID:      teamObj.GetID(),
		Slug:    teamObj.GetSlug(),
		HTMLURL: teamObj.GetHTMLURL(),
	}

	mc.populateLocalCaches(team, members)

	if mc.globalCache != nil {
		mc.globalCache.SetTeamMembers(team, info, members)
	}

	return &cachedTeamData{Info: info, Members: members}, nil
}

// TeamMembersWithDetails returns team members with full metadata (avatars, URLs).
func (mc *GitHubMembershipContext) TeamMembersWithDetails(team string) ([]TeamMember, error) {
	data, err := mc.loadTeamWithMembers(team)
	if err != nil {
		return nil, err
	}
	return data.Members, nil
}

// TeamInfo returns team metadata for display purposes.
func (mc *GitHubMembershipContext) TeamInfo(team string) (*TeamInfo, error) {
	data, err := mc.loadTeamWithMembers(team)
	if err != nil {
		return nil, err
	}
	return data.Info, nil
}
