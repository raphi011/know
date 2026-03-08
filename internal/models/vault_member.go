package models

import (
	"fmt"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// VaultRole defines the permission level for a vault member.
type VaultRole string

const (
	RoleRead  VaultRole = "read"
	RoleWrite VaultRole = "write"
	RoleAdmin VaultRole = "admin"
)

// Level returns the numeric level of a role for comparison.
func (r VaultRole) Level() int {
	switch r {
	case RoleRead:
		return 1
	case RoleWrite:
		return 2
	case RoleAdmin:
		return 3
	default:
		return 0
	}
}

// Valid returns true if the role is a known value.
func (r VaultRole) Valid() bool {
	return r.Level() > 0
}

// AtLeast returns true if this role meets or exceeds the required role.
func (r VaultRole) AtLeast(required VaultRole) bool {
	return r.Level() >= required.Level()
}

// ParseVaultRole validates a string as a VaultRole, returning an error for unknown values.
func ParseVaultRole(s string) (VaultRole, error) {
	r := VaultRole(s)
	if !r.Valid() {
		return "", fmt.Errorf("invalid vault role: %q", s)
	}
	return r, nil
}

// VaultMember represents a user's membership in a vault with a specific role.
type VaultMember struct {
	ID        surrealmodels.RecordID `json:"id"`
	User      surrealmodels.RecordID `json:"user"`
	Vault     surrealmodels.RecordID `json:"vault"`
	Role      string                 `json:"role"`
	CreatedAt time.Time              `json:"created_at"`
}

// VaultPermission is a resolved vault membership for use in auth context.
type VaultPermission struct {
	VaultID string
	Role    VaultRole
}
