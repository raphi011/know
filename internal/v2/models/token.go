package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type APIToken struct {
	ID          surrealmodels.RecordID   `json:"id"`
	User        surrealmodels.RecordID   `json:"user"`
	TokenHash   string                   `json:"token_hash"`
	Name        string                   `json:"name"`
	VaultAccess []surrealmodels.RecordID `json:"vault_access"`
	LastUsed    *time.Time               `json:"last_used,omitempty"`
	ExpiresAt   *time.Time               `json:"expires_at,omitempty"`
	CreatedAt   time.Time                `json:"created_at"`
}
