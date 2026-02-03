package models

import "time"

// ProcedureStep represents a single step within a procedure.
type ProcedureStep struct {
	Order    int    `json:"order"`
	Content  string `json:"content"`
	Optional bool   `json:"optional,omitempty"`
}

// Procedure represents a procedural memory (workflow/process).
type Procedure struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Steps       []ProcedureStep `json:"steps"`
	Embedding   []float32       `json:"embedding,omitempty"`
	Context     *string         `json:"context,omitempty"`
	Labels      []string        `json:"labels,omitempty"`
	Created     time.Time       `json:"created,omitempty"`
	Accessed    time.Time       `json:"accessed,omitempty"`
	AccessCount int             `json:"access_count,omitempty"`
}

// ProcedureSearchResult wraps procedure search results.
type ProcedureSearchResult struct {
	Procedures []Procedure `json:"procedures"`
	Count      int         `json:"count"`
}
