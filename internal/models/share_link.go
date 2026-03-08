package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// ShareLink represents a read-only share link for a specific doc/folder path.
type ShareLink struct {
	ID        surrealmodels.RecordID `json:"id"`
	Vault     surrealmodels.RecordID `json:"vault"`
	TokenHash string                 `json:"token_hash"`
	Path      string                 `json:"path"`
	IsFolder  bool                   `json:"is_folder"`
	CreatedBy surrealmodels.RecordID `json:"created_by"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// IsExpired returns true if the share link has an expiry time that has passed.
func (l *ShareLink) IsExpired() bool {
	return l.ExpiresAt != nil && time.Now().After(*l.ExpiresAt)
}
