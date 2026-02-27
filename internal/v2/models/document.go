package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type DocumentSource string

const (
	SourceManual      DocumentSource = "manual"
	SourceScrape      DocumentSource = "scrape"
	SourceMCP         DocumentSource = "mcp"
	SourceAIGenerated DocumentSource = "ai_generated"
)

type Document struct {
	ID          surrealmodels.RecordID `json:"id"`
	Vault       surrealmodels.RecordID `json:"vault"`
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	ContentBody string                 `json:"content_body"`
	Labels      []string               `json:"labels"`
	DocType     *string                `json:"doc_type,omitempty"`
	Source      DocumentSource         `json:"source"`
	SourcePath  *string                `json:"source_path,omitempty"`
	ContentHash *string                `json:"content_hash,omitempty"`
	Embedding   []float32              `json:"embedding,omitempty"`
	Metadata    map[string]any         `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type DocumentInput struct {
	VaultID     string         `json:"vault_id"`
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	ContentBody string         `json:"content_body"`
	Source      DocumentSource `json:"source"`
	SourcePath  *string        `json:"source_path,omitempty"`
	ContentHash *string        `json:"content_hash,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	DocType     *string        `json:"doc_type,omitempty"`
	Embedding   []float32      `json:"embedding,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Folder is a virtual folder derived from document paths.
type Folder struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	DocCount int    `json:"doc_count"`
}
