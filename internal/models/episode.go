package models

import "time"

// Episode represents an episodic memory (conversation segment).
type Episode struct {
	ID             string         `json:"id"`
	Content        string         `json:"content"`
	Summary        *string        `json:"summary,omitempty"`
	Embedding      []float32      `json:"embedding,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Timestamp      time.Time      `json:"timestamp,omitempty"`
	Context        *string        `json:"context,omitempty"`
	Created        time.Time      `json:"created,omitempty"`
	Accessed       time.Time      `json:"accessed,omitempty"`
	AccessCount    int            `json:"access_count,omitempty"`
	LinkedEntities int            `json:"linked_entities,omitempty"`
	Entities       []Entity       `json:"entities,omitempty"`
}

// EpisodeSearchResult wraps episode search results.
type EpisodeSearchResult struct {
	Episodes []Episode `json:"episodes"`
	Count    int       `json:"count"`
}
