// Copyright 2023 Palantir Technologies, Inc.
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
	"fmt"
	"time"

	"github.com/hairyhenderson/go-codeowners"
	lru "github.com/hashicorp/golang-lru"
)

const DefaultMembershipTTL = 5 * time.Minute

// TeamMember represents a GitHub user with basic metadata
type TeamMember struct {
	Login     string
	AvatarURL string
	HTMLURL   string
}

// TeamInfo represents team metadata
type TeamInfo struct {
	ID      int64
	Slug    string
	HTMLURL string
}

// GlobalCache implementations provide a way to cache values that are safe to
// cache at the application level. Values in the global cache should not become
// stale due to external changes and should only expire to prevent the cache
// from becoming infinitely large.
type GlobalCache interface {
	GetPushedAt(repoID int64, sha string) (time.Time, bool)
	SetPushedAt(repoID int64, sha string, t time.Time)

	// GetCodeowners returns the cached parsed CODEOWNERS for a repository at a
	// specific base branch commit. Since commit SHAs are immutable, caching the
	// parsed content is safe and avoids repeated HTTP requests.
	GetCodeowners(repoID int64, baseRefOID string) (*codeowners.Codeowners, bool)
	SetCodeowners(repoID int64, baseRefOID string, co *codeowners.Codeowners)

	// GetTeamMembership returns cached team membership status.
	// Returns (isMember, found). If found is false, the value is not in cache.
	GetTeamMembership(team, user string) (bool, bool)
	SetTeamMembership(team, user string, isMember bool)

	// GetTeamMembers returns cached team members with metadata.
	// Returns (members, info, found). If found is false, the value is not in cache.
	GetTeamMembers(team string) ([]TeamMember, *TeamInfo, bool)
	SetTeamMembers(team string, info *TeamInfo, members []TeamMember)
}

type membershipEntry struct {
	isMember  bool
	expiresAt time.Time
}

type teamMembersEntry struct {
	info      *TeamInfo
	members   []TeamMember
	expiresAt time.Time
}

// LRUGlobalCache is a GlobalCache where each data type is stored in a separate
// LRU cache. This prevents frequently used data of one type from evicting less
// frequently used data of a different type.
type LRUGlobalCache struct {
	pushedAt   *lru.Cache
	codeowners *lru.Cache
	membership *lru.Cache
	teams      *lru.Cache
	memberTTL  time.Duration
}

func NewLRUGlobalCache(pushedAtSize, codeownersSize, membershipSize, teamsSize int) (*LRUGlobalCache, error) {
	pushedAt, err := lru.New(pushedAtSize)
	if err != nil {
		return nil, err
	}
	codeownersCache, err := lru.New(codeownersSize)
	if err != nil {
		return nil, err
	}
	membership, err := lru.New(membershipSize)
	if err != nil {
		return nil, err
	}
	teams, err := lru.New(teamsSize)
	if err != nil {
		return nil, err
	}
	return &LRUGlobalCache{
		pushedAt:   pushedAt,
		codeowners: codeownersCache,
		membership: membership,
		teams:      teams,
		memberTTL:  DefaultMembershipTTL,
	}, nil
}

func (c *LRUGlobalCache) GetPushedAt(repoID int64, sha string) (time.Time, bool) {
	if val, ok := c.pushedAt.Get(pushedAtKey(repoID, sha)); ok {
		if t, ok := val.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c *LRUGlobalCache) SetPushedAt(repoID int64, sha string, t time.Time) {
	c.pushedAt.Add(pushedAtKey(repoID, sha), t)
}

func pushedAtKey(repoID int64, sha string) string {
	return fmt.Sprintf("%d:%s", repoID, sha)
}

func (c *LRUGlobalCache) GetCodeowners(repoID int64, baseRefOID string) (*codeowners.Codeowners, bool) {
	key := fmt.Sprintf("%d:%s", repoID, baseRefOID)
	if val, ok := c.codeowners.Get(key); ok {
		if co, ok := val.(*codeowners.Codeowners); ok {
			return co, true
		}
	}
	return nil, false
}

func (c *LRUGlobalCache) SetCodeowners(repoID int64, baseRefOID string, co *codeowners.Codeowners) {
	key := fmt.Sprintf("%d:%s", repoID, baseRefOID)
	c.codeowners.Add(key, co)
}

func (c *LRUGlobalCache) GetTeamMembership(team, user string) (bool, bool) {
	key := team + ":" + user
	if val, ok := c.membership.Get(key); ok {
		if entry, ok := val.(membershipEntry); ok {
			if time.Now().Before(entry.expiresAt) {
				return entry.isMember, true
			}
			// Expired - remove and return not found
			c.membership.Remove(key)
		}
	}
	return false, false
}

func (c *LRUGlobalCache) SetTeamMembership(team, user string, isMember bool) {
	key := team + ":" + user
	c.membership.Add(key, membershipEntry{
		isMember:  isMember,
		expiresAt: time.Now().Add(c.memberTTL),
	})
}

func (c *LRUGlobalCache) GetTeamMembers(team string) ([]TeamMember, *TeamInfo, bool) {
	if val, ok := c.teams.Get(team); ok {
		if entry, ok := val.(teamMembersEntry); ok {
			if time.Now().Before(entry.expiresAt) {
				return entry.members, entry.info, true
			}
			// Expired - remove and return not found
			c.teams.Remove(team)
		}
	}
	return nil, nil, false
}

func (c *LRUGlobalCache) SetTeamMembers(team string, info *TeamInfo, members []TeamMember) {
	c.teams.Add(team, teamMembersEntry{
		info:      info,
		members:   members,
		expiresAt: time.Now().Add(c.memberTTL),
	})
}
