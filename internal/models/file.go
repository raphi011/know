package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// FilenameStem returns the normalized filename without extension for a path.
// Normalization: lowercase, spaces and underscores replaced with hyphens.
// Returns "" for folder paths (trailing slash).
func FilenameStem(path string) string {
	if strings.HasSuffix(path, "/") {
		return ""
	}
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.ToLower(stem)
	stem = strings.ReplaceAll(stem, " ", "-")
	stem = strings.ReplaceAll(stem, "_", "-")
	return stem
}

// File is a unified entity representing any path-addressable item in a vault:
// markdown documents, binary files (images, PDFs, audio), and folders.
type File struct {
	ID             surrealmodels.RecordID `json:"id"`
	Vault          surrealmodels.RecordID `json:"vault"`
	Path           string                 `json:"path"`
	Title          string                 `json:"title"`
	Stem           string                 `json:"stem"`
	IsFolder       bool                   `json:"is_folder"`
	NoEmbed        bool                   `json:"no_embed"`
	MimeType       string                 `json:"mime_type"`
	ContentLength  int                    `json:"content_length"`
	ContentHash    *string                `json:"content_hash,omitempty"`
	Labels         []string               `json:"labels"`
	DocType        *string                `json:"doc_type,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	Size           int                    `json:"size"`
	LastAccessedAt *time.Time             `json:"last_accessed_at,omitempty"`
	AccessCount    int                    `json:"access_count"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// FileInput holds the data needed to create or update a file.
type FileInput struct {
	VaultID       string         `json:"vault_id"`
	Path          string         `json:"path"`
	Title         string         `json:"title"`
	Content       string         `json:"content"` // text content (stored in blob store, not DB)
	ContentHash   *string        `json:"content_hash,omitempty"`
	ContentLength int            `json:"content_length,omitempty"` // pre-computed content length for DB
	Labels        []string       `json:"labels,omitempty"`
	DocType       *string        `json:"doc_type,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	MimeType      string         `json:"mime_type"`
	Data          []byte         `json:"data,omitempty"`
	Size          int            `json:"size,omitempty"` // explicit size override (e.g. streaming imports where Data is not buffered)
	IsFolder      bool           `json:"is_folder"`
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
	Title       string    `json:"title"`
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
	Title string `json:"title,omitempty"`
	IsDir bool   `json:"isDir"`
	Size  int    `json:"size,omitempty"`
}
