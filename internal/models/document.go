package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Document struct {
	ID             surrealmodels.RecordID `json:"id"`
	Vault          surrealmodels.RecordID `json:"vault"`
	Path           string                 `json:"path"`
	Title          string                 `json:"title"`
	Content        string                 `json:"content"`
	ContentBody    string                 `json:"content_body"`
	ContentLength  int                    `json:"content_length"`
	Labels         []string               `json:"labels"`
	DocType        *string                `json:"doc_type,omitempty"`
	ContentHash    *string                `json:"content_hash,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	Processed      bool                   `json:"processed"`
	LastAccessedAt *time.Time             `json:"last_accessed_at,omitempty"`
	AccessCount    int                    `json:"access_count"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type DocumentInput struct {
	VaultID     string         `json:"vault_id"`
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	ContentBody string         `json:"content_body"`
	ContentHash *string        `json:"content_hash,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	DocType     *string        `json:"doc_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// DocumentMeta is a lightweight projection of a document for metadata-only
// operations (e.g. WebDAV Stat, directory listings) that don't need content.
type DocumentMeta struct {
	Path          string    `json:"path"`
	ContentLength int       `json:"content_length"`
	ContentHash   *string   `json:"content_hash,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LabelCount holds a label and its document count.
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// FileEntry is a lightweight entry for directory listings (ls endpoint).
type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int    `json:"size,omitempty"`
}

// Folder is a first-class folder record backed by the folder table.
type Folder struct {
	ID        surrealmodels.RecordID `json:"id"`
	Vault     surrealmodels.RecordID `json:"vault"`
	Path      string                 `json:"path"`
	Name      string                 `json:"name"`
	CreatedAt time.Time              `json:"created_at"`
}
