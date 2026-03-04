package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// DocumentVersion represents a historical snapshot of a document's content.
// Versions store the OLD content that was replaced during an update.
type DocumentVersion struct {
	ID          surrealmodels.RecordID `json:"id"`
	Document    surrealmodels.RecordID `json:"document"`
	Vault       surrealmodels.RecordID `json:"vault"`
	Version     int                    `json:"version"`
	Content     string                 `json:"content"`
	ContentHash string                 `json:"content_hash"`
	Title       string                 `json:"title"`
	Source      DocumentSource         `json:"source"`
	CreatedAt   time.Time              `json:"created_at"`
}

// DocumentVersionInput holds the data needed to create a version snapshot.
type DocumentVersionInput struct {
	DocumentID  string
	VaultID     string
	Content     string
	ContentHash string
	Title       string
	Source      DocumentSource
}
