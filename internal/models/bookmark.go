package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Bookmark represents a user's pinned file or folder for quick access.
type Bookmark struct {
	ID        surrealmodels.RecordID `json:"id"`
	User      surrealmodels.RecordID `json:"user"`
	File      surrealmodels.RecordID `json:"file"`
	Vault     surrealmodels.RecordID `json:"vault"`
	CreatedAt time.Time              `json:"created_at"`
}
