package models

import (
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Chunk struct {
	ID        surrealmodels.RecordID `json:"id"`
	File      surrealmodels.RecordID `json:"file"`
	Text      string                 `json:"text"`
	MimeType  string                 `json:"mime_type"`
	Position  int                    `json:"position"`
	SourceLoc *string                `json:"source_loc,omitempty"`
	DataHash  *string                `json:"data_hash,omitempty"`
	Labels    []string               `json:"labels"`
	Embedding []float32              `json:"embedding"`
}

// IsMultimodal returns true if this chunk has binary data (e.g. a PDF page image)
// that should be embedded via the multimodal embedder rather than text-only.
func (c Chunk) IsMultimodal() bool {
	return c.DataHash != nil && c.MimeType != "" && c.MimeType != "text/plain"
}

type ChunkInput struct {
	FileID    string    `json:"file_id"`
	Text      string    `json:"text"`
	MimeType  string    `json:"mime_type"`
	Position  int       `json:"position"`
	SourceLoc *string   `json:"source_loc,omitempty"`
	DataHash  *string   `json:"data_hash,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
	Embedding []float32 `json:"embedding"`
}
