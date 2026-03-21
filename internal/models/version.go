package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// FileVersion represents a historical snapshot of a file's content.
// Content is stored in the blob store, referenced by Hash.
type FileVersion struct {
	ID        surrealmodels.RecordID `json:"id"`
	File      surrealmodels.RecordID `json:"file"`
	Vault     surrealmodels.RecordID `json:"vault"`
	Version   int                    `json:"version"`
	Hash      string                 `json:"hash"`
	Title     string                 `json:"title"`
	CreatedAt time.Time              `json:"created_at"`
}

// FileVersionInput holds the data needed to create a version snapshot.
type FileVersionInput struct {
	FileID  string
	VaultID string
	Hash    string
	Title   string
}
