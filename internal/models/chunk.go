package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Chunk struct {
	ID        surrealmodels.RecordID `json:"id"`
	File      surrealmodels.RecordID `json:"file"`
	Text      string                 `json:"text"`
	MimeType  string                 `json:"mime_type"`
	Position  int                    `json:"position"`
	SourceLoc *string                `json:"source_loc,omitempty"`
	Labels    []string               `json:"labels"`
	Embedding []float32              `json:"embedding"`
	EmbedAt   *time.Time             `json:"embed_at,omitempty"`
}

type ChunkInput struct {
	FileID    string     `json:"file_id"`
	Text      string     `json:"text"`
	MimeType  string     `json:"mime_type"`
	Position  int        `json:"position"`
	SourceLoc *string    `json:"source_loc,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	Embedding []float32  `json:"embedding"`
	EmbedAt   *time.Time `json:"embed_at,omitempty"`
}
