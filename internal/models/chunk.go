package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Chunk struct {
	ID          surrealmodels.RecordID `json:"id"`
	Document    surrealmodels.RecordID `json:"document"`
	Content     string                 `json:"content"`
	Position    int                    `json:"position"`
	HeadingPath *string                `json:"heading_path,omitempty"`
	Labels      []string               `json:"labels"`
	Embedding   []float32              `json:"embedding"`
	EmbedAt     *time.Time             `json:"embed_at,omitempty"`
}

type ChunkInput struct {
	DocumentID  string     `json:"document_id"`
	Content     string     `json:"content"`
	Position    int        `json:"position"`
	HeadingPath *string    `json:"heading_path,omitempty"`
	Labels      []string   `json:"labels,omitempty"`
	Embedding   []float32  `json:"embedding"`
	EmbedAt     *time.Time `json:"embed_at,omitempty"`
}
