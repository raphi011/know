package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Chunk represents a RAG piece extracted from long entity content.
// Auto-generated when entity content exceeds ChunkThreshold.
type Chunk struct {
	ID surrealmodels.RecordID `json:"id"`

	// Parent reference
	Entity surrealmodels.RecordID `json:"entity"`

	// Content
	Content     string  `json:"content"`                   // Chunk text
	Position    int     `json:"position"`                  // Order within entity
	HeadingPath *string `json:"heading_path,omitempty"`    // "## Setup > ### Install"

	// Organization (inherited from parent)
	Labels []string `json:"labels"`

	// Search
	Embedding []float32 `json:"embedding"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// ChunkInput is the input structure for creating chunks.
type ChunkInput struct {
	EntityID    string    `json:"entity_id"`
	Content     string    `json:"content"`
	Position    int       `json:"position"`
	HeadingPath *string   `json:"heading_path,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	Embedding   []float32 `json:"embedding"`
}

// ChunkingConfig defines parameters for content chunking.
type ChunkingConfig struct {
	// Threshold is the minimum content length (chars) to trigger chunking.
	// Content shorter than this is not chunked.
	Threshold int

	// TargetSize is the target chunk size in characters.
	TargetSize int

	// MinSize is the minimum chunk size. Chunks smaller than this
	// are merged with adjacent chunks.
	MinSize int

	// MaxSize is the maximum chunk size. Chunks larger than this
	// are split at sentence boundaries.
	MaxSize int

	// Overlap is the character overlap between adjacent chunks.
	Overlap int
}

// DefaultChunkingConfig returns the default chunking configuration.
func DefaultChunkingConfig() ChunkingConfig {
	return ChunkingConfig{
		Threshold:  1500,  // Only chunk content > 1500 chars
		TargetSize: 750,   // Target ~750 chars per chunk
		MinSize:    200,   // Don't create tiny chunks
		MaxSize:    1000,  // Cap at 1000 chars
		Overlap:    100,   // 100 char overlap for context
	}
}
