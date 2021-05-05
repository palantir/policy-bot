package pull

import (
	"fmt"
	"strings"
)

type RepositoryPermission uint8

const (
	PermissionNone RepositoryPermission = iota
	PermissionRead
	PermissionTriage
	PermissionWrite
	PermissionMaintain
	PermissionAdmin
)

func ParsePermission(p string) (RepositoryPermission, error) {
	switch strings.ToLower(p) {
	case "read":
		return PermissionRead, nil
	case "triage":
		return PermissionTriage, nil
	case "write":
		return PermissionWrite, nil
	case "maintain":
		return PermissionMaintain, nil
	case "admin":
		return PermissionAdmin, nil
	}
	return PermissionNone, fmt.Errorf("invalid permission: %s", p)
}

func (p RepositoryPermission) String() string {
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
