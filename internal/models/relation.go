package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// RelationSource indicates how a relation was created.
type RelationSource string

const (
	RelationSourceManual     RelationSource = "manual"      // User explicitly created
	RelationSourceInferred   RelationSource = "inferred"    // Detected during ingestion (markdown links, mentions)
	RelationSourceAIDetected RelationSource = "ai_detected" // LLM found semantic relationship
)

// Relation represents a relationship between two entities in the knowledge graph.
type Relation struct {
	ID surrealmodels.RecordID `json:"id"`

	// Direction
	In  surrealmodels.RecordID `json:"in"`  // Source entity
	Out surrealmodels.RecordID `json:"out"` // Target entity

	// Properties
	RelType  string  `json:"rel_type"` // "works_on", "owns", "references", etc.
	Strength float64 `json:"strength"` // Relationship strength (0-1)
	Source   string  `json:"source"`   // "manual" | "inferred" | "ai_detected"

	// Additional context
	Metadata map[string]any `json:"metadata,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// RelationInput is the input structure for creating relations.
type RelationInput struct {
	FromID   string         `json:"from_id"`
	ToID     string         `json:"to_id"`
	RelType  string         `json:"rel_type"`
	Strength *float64       `json:"strength,omitempty"` // Default 1.0
	Source   *string        `json:"source,omitempty"`   // Default "manual"
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Contradiction represents a detected conflict between two entities.
type Contradiction struct {
	ID surrealmodels.RecordID `json:"id"`

	// Direction
	In  surrealmodels.RecordID `json:"in"`  // First entity
	Out surrealmodels.RecordID `json:"out"` // Second entity

	// Details
	Explanation string  `json:"explanation"` // What contradicts
	Confidence  float64 `json:"confidence"`  // How certain
	Resolved    bool    `json:"resolved"`    // Has been addressed?

	// Timestamps
	DetectedAt time.Time `json:"detected_at"`
}

// ContradictionInput is the input structure for creating contradictions.
type ContradictionInput struct {
	FromID      string   `json:"from_id"`
	ToID        string   `json:"to_id"`
	Explanation string   `json:"explanation"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

// TokenUsage tracks LLM token consumption for cost monitoring.
type TokenUsage struct {
	ID surrealmodels.RecordID `json:"id"`

	Operation    string   `json:"operation"`              // "embed", "ask", "extract_graph", "render"
	Model        string   `json:"model"`                  // "gpt-4", "claude-3", "ollama/llama3"
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	TotalTokens  int      `json:"total_tokens"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`     // Estimated cost if known
	EntityID     *string  `json:"entity_id,omitempty"`    // Related entity if applicable

	CreatedAt time.Time `json:"created_at"`
}

// TokenUsageInput is the input structure for recording token usage.
type TokenUsageInput struct {
	Operation    string   `json:"operation"`
	Model        string   `json:"model"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	EntityID     *string  `json:"entity_id,omitempty"`
}

// TokenUsageSummary provides aggregated token usage statistics.
type TokenUsageSummary struct {
	TotalTokens      int              `json:"total_tokens"`
	TotalCostUSD     float64          `json:"total_cost_usd"`
	ByOperation      map[string]int   `json:"by_operation"`       // operation -> token count
	ByModel          map[string]int   `json:"by_model"`           // model -> token count
	OperationPercent map[string]float64 `json:"operation_percent"` // operation -> percentage
}
