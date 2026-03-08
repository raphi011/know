package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type APIToken struct {
	ID        surrealmodels.RecordID `json:"id"`
	User      surrealmodels.RecordID `json:"user"`
	TokenHash string                 `json:"token_hash"`
	Name      string                 `json:"name"`
	LastUsed  *time.Time             `json:"last_used,omitempty"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// IsExpired returns true if the token has an expiry time that has passed.
func (t *APIToken) IsExpired() bool {
	return t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt)
}
