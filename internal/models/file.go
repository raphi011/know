package models

import (
	"fmt"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// File is a unified entity representing any path-addressable item in a vault:
// markdown documents, binary files (images, PDFs, audio), and folders.
type File struct {
	ID             surrealmodels.RecordID `json:"id"`
	Vault          surrealmodels.RecordID `json:"vault"`
	Path           string                 `json:"path"`
	Title          string                 `json:"title"`
	IsFolder       bool                   `json:"is_folder"`
	MimeType       string                 `json:"mime_type"`
	Content        string                 `json:"content"`
	ContentLength  int                    `json:"content_length"`
	ContentHash    *string                `json:"content_hash,omitempty"`
	Labels         []string               `json:"labels"`
	DocType        *string                `json:"doc_type,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	Data           []byte                 `json:"data,omitempty"`
	Size           int                    `json:"size"`
	Processed      bool                   `json:"processed"`
	LastAccessedAt *time.Time             `json:"last_accessed_at,omitempty"`
	AccessCount    int                    `json:"access_count"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// FileInput holds the data needed to create or update a file.
type FileInput struct {
	VaultID     string         `json:"vault_id"`
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	ContentHash *string        `json:"content_hash,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	DocType     *string        `json:"doc_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	MimeType    string         `json:"mime_type"`
	Data        []byte         `json:"data,omitempty"`
	IsFolder    bool           `json:"is_folder"`
}

// Validate checks that the FileInput has consistent fields for its kind.
func (f FileInput) Validate() error {
	if f.Path == "" {
		return fmt.Errorf("path is required")
	}
	if f.IsFolder {
		if f.Content != "" || len(f.Data) > 0 {
			return fmt.Errorf("folders must not have content or data")
		}
	}
	return nil
}

// FileMeta is a lightweight projection for metadata-only operations
// (e.g. WebDAV Stat, directory listings) that don't need content or data.
type FileMeta struct {
	Path        string    `json:"path"`
	MimeType    string    `json:"mime_type"`
	Size        int       `json:"size"`
	ContentHash *string   `json:"content_hash,omitempty"`
	IsFolder    bool      `json:"is_folder"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LabelCount holds a label and its file count.
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
