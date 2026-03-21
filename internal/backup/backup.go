// Package backup provides manifest-based export and restore for vaults.
// Exports produce a tar.gz archive containing raw files in their original
// path structure plus a JSON manifest with metadata, folder settings, and
// version history. Chunks and embeddings are omitted — they are regenerated
// on restore via the normal pipeline.
package backup

import (
	"time"

	"github.com/raphi011/know/internal/models"
)

// ManifestVersion is the current manifest format version.
const ManifestVersion = 1

// Manifest describes a vault backup.
type Manifest struct {
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exported_at"`
	Vault      VaultInfo     `json:"vault"`
	Files      []FileInfo    `json:"files"`
	Folders    []FolderInfo  `json:"folders"`
	Versions   []VersionInfo `json:"versions,omitempty"`
}

// VaultInfo captures vault identity and settings.
type VaultInfo struct {
	Name        string                `json:"name"`
	Description *string               `json:"description,omitempty"`
	Settings    *models.VaultSettings `json:"settings,omitempty"`
}

// FileInfo captures file metadata (content is stored as a blob in the archive).
type FileInfo struct {
	Path      string         `json:"path"`
	Title     string         `json:"title"`
	Hash      string         `json:"hash"`
	Size      int            `json:"size"`
	Labels    []string       `json:"labels,omitempty"`
	DocType   *string        `json:"doc_type,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	MimeType  string         `json:"mime_type"`
	NoEmbed   bool           `json:"no_embed,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FolderInfo captures folder metadata and settings.
type FolderInfo struct {
	Path    string `json:"path"`
	NoEmbed bool   `json:"no_embed,omitempty"`
}

// VersionInfo captures a historical version snapshot.
// Content is stored in the archive under versions/<sharded-hash>.
type VersionInfo struct {
	FilePath  string    `json:"file_path"`
	Version   int       `json:"version"`
	Hash      string    `json:"hash"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}
