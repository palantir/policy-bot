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
