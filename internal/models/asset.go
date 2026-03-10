package models

import (
	"path"
	"strings"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Asset struct {
	ID          surrealmodels.RecordID `json:"id"`
	Vault       surrealmodels.RecordID `json:"vault"`
	Path        string                 `json:"path"`
	MimeType    string                 `json:"mime_type"`
	Size        int                    `json:"size"`
	ContentHash string                 `json:"content_hash"`
	Data        []byte                 `json:"data"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// AssetMeta is a lightweight projection of an asset without binary data.
type AssetMeta struct {
	Path        string    `json:"path"`
	MimeType    string    `json:"mime_type"`
	Size        int       `json:"size"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AssetInput struct {
	VaultID  string `json:"vault_id"`
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

// imageExtensions maps lowercase extensions to MIME types.
var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
}

// IsImageFile returns true if the file has a supported image extension.
func IsImageFile(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	_, ok := imageExtensions[ext]
	return ok
}

// MimeTypeFromExt returns the MIME type for a file based on its extension.
// Returns empty string for unsupported extensions.
func MimeTypeFromExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	return imageExtensions[ext]
}
