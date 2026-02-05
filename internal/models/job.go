// Package models defines data structures for the Knowhow knowledge database.
package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// IngestJob represents a persisted async ingestion job.
type IngestJob struct {
	ID          surrealmodels.RecordID `json:"id"`
	JobType     string                 `json:"job_type"`
	Status      string                 `json:"status"`
	Name        *string                `json:"name,omitempty"`   // User-provided name for rerunning
	Labels      []string               `json:"labels,omitempty"` // Curated labels applied to entities
	DirPath     string                 `json:"dir_path"`
	Files       []string               `json:"files"`
	Options     map[string]any         `json:"options,omitempty"`
	Total       int                    `json:"total"`
	Progress    int                    `json:"progress"`
	Result      map[string]any         `json:"result,omitempty"`
	Error       *string                `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}
