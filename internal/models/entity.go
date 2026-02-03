package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Entity represents a memory entity in the knowledge graph.
// Matches Python memcp/models.py EntityResult.
type Entity struct {
	ID             surrealmodels.RecordID `json:"id"`
	Type           string    `json:"type,omitempty"`
	Labels         []string  `json:"labels,omitempty"`
	Content        string    `json:"content"`
	Embedding      []float32 `json:"embedding,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	Source         *string   `json:"source,omitempty"`
	DecayWeight    float64   `json:"decay_weight,omitempty"`
	Context        *string   `json:"context,omitempty"`
	Importance     float64   `json:"importance,omitempty"`
	UserImportance *float64  `json:"user_importance,omitempty"`
	Created        time.Time `json:"created,omitempty"`
	Accessed       time.Time `json:"accessed,omitempty"`
	AccessCount    int       `json:"access_count,omitempty"`
}

// SearchResult wraps entity search results.
type SearchResult struct {
	Entities []Entity `json:"entities"`
	Count    int      `json:"count"`
	Summary  *string  `json:"summary,omitempty"`
}
