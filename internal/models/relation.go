package models

import "time"

// Relation represents a relationship between entities.
type Relation struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	RelType string    `json:"rel_type"`
	Weight  float64   `json:"weight,omitempty"`
	Created time.Time `json:"created,omitempty"`
}
