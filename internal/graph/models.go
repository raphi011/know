// Package graph provides GraphQL types and resolvers for Knowhow.
package graph

import (
	"time"
)

// Entity represents a knowledge entity in the GraphQL schema.
type Entity struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Content     *string        `json:"content,omitempty"`
	Summary     *string        `json:"summary,omitempty"`
	Labels      []string       `json:"labels"`
	ContentHash *string        `json:"contentHash,omitempty"`
	Verified    bool           `json:"verified"`
	Confidence  float64        `json:"confidence"`
	Source      string         `json:"source"`
	SourcePath  *string        `json:"sourcePath,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	AccessedAt  time.Time      `json:"accessedAt"`
	AccessCount int            `json:"accessCount"`
	Relations   []Relation     `json:"relations"`
}

// Relation represents a relationship between entities.
type Relation struct {
	ID        string    `json:"id"`
	FromID    string    `json:"fromId"`
	ToID      string    `json:"toId"`
	RelType   string    `json:"relType"`
	Strength  float64   `json:"strength"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

// Template represents an output rendering template.
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// EntitySearchResult wraps search results with match context.
type EntitySearchResult struct {
	Entity        Entity       `json:"entity"`
	MatchedChunks []ChunkMatch `json:"matchedChunks"`
	Score         float64      `json:"score"`
}

// ChunkMatch represents a matching chunk within a search result.
type ChunkMatch struct {
	Content     string  `json:"content"`
	HeadingPath *string `json:"headingPath,omitempty"`
	Position    int     `json:"position"`
}

// IngestResult summarizes an ingestion operation.
type IngestResult struct {
	FilesProcessed   int      `json:"filesProcessed"`
	FilesSkipped     int      `json:"filesSkipped"`
	EntitiesCreated  int      `json:"entitiesCreated"`
	ChunksCreated    int      `json:"chunksCreated"`
	RelationsCreated int      `json:"relationsCreated"`
	Errors           []string `json:"errors"`
}

// LabelCount represents a label with its entity count.
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// TypeCount represents an entity type with its count.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// TokenUsageSummary provides aggregated token usage statistics.
type TokenUsageSummary struct {
	TotalTokens  int            `json:"totalTokens"`
	TotalCostUSD float64        `json:"totalCostUSD"`
	ByOperation  map[string]any `json:"byOperation"`
	ByModel      map[string]any `json:"byModel"`
}

// Conversation represents a chat session in the GraphQL schema.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	EntityID  *string   `json:"entityId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Messages  []Message `json:"messages"`
}

// Message represents a chat message in the GraphQL schema.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChatMessageInput is the input for multi-turn chat history.
type ChatMessageInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// EntityInput is the input for creating entities.
type EntityInput struct {
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Content    *string        `json:"content,omitempty"`
	Summary    *string        `json:"summary,omitempty"`
	Labels     []string       `json:"labels,omitempty"`
	Verified   *bool          `json:"verified,omitempty"`
	Source     *string        `json:"source,omitempty"`
	SourcePath *string        `json:"sourcePath,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// EntityUpdate is the input for updating entities.
type EntityUpdate struct {
	Name      *string        `json:"name,omitempty"`
	Content   *string        `json:"content,omitempty"`
	Summary   *string        `json:"summary,omitempty"`
	Labels    []string       `json:"labels,omitempty"`
	AddLabels []string       `json:"addLabels,omitempty"`
	DelLabels []string       `json:"delLabels,omitempty"`
	Verified  *bool          `json:"verified,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// RelationInput is the input for creating relations.
type RelationInput struct {
	FromID   string   `json:"fromId"`
	ToID     string   `json:"toId"`
	RelType  string   `json:"relType"`
	Strength *float64 `json:"strength,omitempty"`
}

// SearchInput is the input for search operations.
type SearchInput struct {
	Query        string   `json:"query"`
	Labels       []string `json:"labels,omitempty"`
	Types        []string `json:"types,omitempty"`
	VerifiedOnly *bool    `json:"verifiedOnly,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
}

// IngestInput is the input for ingest operations.
type IngestInput struct {
	// User-provided name for identifying and rerunning this job
	Name *string `json:"name,omitempty"`
	// Curated labels to apply to all ingested entities
	Labels       []string `json:"labels,omitempty"`
	ExtractGraph *bool    `json:"extractGraph,omitempty"`
	DryRun       *bool    `json:"dryRun,omitempty"`
	Recursive    *bool    `json:"recursive,omitempty"`
}
