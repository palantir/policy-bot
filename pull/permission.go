// Copyright 2021 Palantir Technologies, Inc.
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
	"strings"
)

type Permission uint8

const (
	PermissionNone Permission = iota
	PermissionRead
	PermissionTriage
	PermissionWrite
	PermissionMaintain
	PermissionAdmin
)

func ParsePermission(s string) (Permission, error) {
	var p Permission
	err := p.UnmarshalText([]byte(s))
	return p, err
}

func ParsePermissionMap(m map[string]bool) Permission {
	switch {
	case m["admin"]:
		return PermissionAdmin
	case m["maintain"]:
		return PermissionMaintain
	case m["write"] || m["push"]:
		return PermissionWrite
	case m["triage"]:
		return PermissionTriage
	case m["read"] || m["pull"]:
		return PermissionRead
	}
	return PermissionNone
}

func (p Permission) String() string {
	switch p {
	case PermissionNone:
		return "none"
	case PermissionRead:
		return "read"
	case PermissionTriage:
		return "triage"
	case PermissionWrite:
		return "write"
	case PermissionMaintain:
		return "maintain"
	case PermissionAdmin:
		return "admin"
	}
	return fmt.Sprintf("unknown(%d)", p)
}

func (p Permission) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Permission) UnmarshalText(text []byte) error {
	switch strings.ToLower(string(text)) {
	case "none":
		*p = PermissionNone
	case "read":
		*p = PermissionRead
	case "triage":
		*p = PermissionTriage
	case "write":
		*p = PermissionWrite
	case "maintain":
		*p = PermissionMaintain
	case "admin":
		*p = PermissionAdmin
	}
	return fmt.Errorf("invalid permission: %s", text)
}
