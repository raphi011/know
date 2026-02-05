// Package models defines data structures for the Knowhow knowledge database.
package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// EntitySource indicates how an entity was created.
type EntitySource string

const (
	SourceManual      EntitySource = "manual"       // User created via CLI/TUI
	SourceMCP         EntitySource = "mcp"          // Claude created via MCP tools
	SourceScrape      EntitySource = "scrape"       // Ingested from Markdown files
	SourceAIGenerated EntitySource = "ai_generated" // LLM synthesized content
)

// Entity represents a flexible knowledge atom in the knowledge graph.
// Can be any type: person, service, document, concept, task, etc.
type Entity struct {
	ID surrealmodels.RecordID `json:"id"`

	// Identity
	Type string `json:"type"`          // "person", "service", "document", "concept", "task", etc.
	Name string `json:"name"`          // Display name/title

	// Content (optional - not all entities need long content)
	Content *string `json:"content,omitempty"` // Full text (Markdown)
	Summary *string `json:"summary,omitempty"` // Short description

	// Organization
	Labels []string `json:"labels"` // Flexible tags ["work", "banking", "team-platform"]

	// Content Hash (for skip-unchanged deduplication)
	ContentHash *string `json:"content_hash,omitempty"` // SHA256 of raw file bytes

	// Quality & Trust
	Verified   bool         `json:"verified"`   // Human-reviewed?
	Confidence float64      `json:"confidence"` // 0-1 certainty (for AI content)
	Source     EntitySource `json:"source"`     // "manual" | "mcp" | "scrape" | "ai_generated"
	SourcePath *string      `json:"source_path,omitempty"` // Original file path if scraped

	// Type-specific data
	Metadata map[string]any `json:"metadata,omitempty"`

	// Search
	Embedding []float32 `json:"embedding,omitempty"`

	// Timestamps
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Accessed    time.Time `json:"accessed"`
	AccessCount int       `json:"access_count"`
}

// EntityInput is the input structure for creating/updating entities.
// Uses pointers for optional fields to distinguish between unset and empty.
type EntityInput struct {
	// ID is an optional explicit entity ID. If provided, used instead of slugified name.
	// Useful for ensuring unique IDs when scraping files (e.g., from relative path).
	ID          *string        `json:"id,omitempty"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Content     *string        `json:"content,omitempty"`
	Summary     *string        `json:"summary,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	ContentHash *string        `json:"content_hash,omitempty"`
	Verified    *bool          `json:"verified,omitempty"`
	Confidence  *float64       `json:"confidence,omitempty"`
	Source      *EntitySource  `json:"source,omitempty"`
	SourcePath  *string        `json:"source_path,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Embedding   []float32      `json:"embedding,omitempty"`
}

// EntityUpdate is the input structure for partial entity updates.
// All fields are optional - only non-nil fields will be updated.
type EntityUpdate struct {
	Name       *string           `json:"name,omitempty"`
	Content    *string           `json:"content,omitempty"`
	Summary    *string           `json:"summary,omitempty"`
	Labels     []string          `json:"labels,omitempty"`     // Replace labels
	AddLabels  []string          `json:"add_labels,omitempty"` // Add to existing
	DelLabels  []string          `json:"del_labels,omitempty"` // Remove from existing
	Verified   *bool             `json:"verified,omitempty"`
	Confidence *float64          `json:"confidence,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Embedding  []float32         `json:"embedding,omitempty"`
}

// EntitySearchResult wraps entity search results with match context.
type EntitySearchResult struct {
	Entity
	MatchedChunks []ChunkMatch `json:"matched_chunks,omitempty"` // If search hit chunks
	Score         float64      `json:"score,omitempty"`          // Relevance score
}

// ChunkMatch represents a matching chunk within a search result.
type ChunkMatch struct {
	Content     string  `json:"content"`
	HeadingPath *string `json:"heading_path,omitempty"`
	Position    int     `json:"position"`
	Score       float64 `json:"score,omitempty"`
}
