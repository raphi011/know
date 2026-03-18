package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type PipelineJob struct {
	ID          surrealmodels.RecordID `json:"id"`
	File        surrealmodels.RecordID `json:"file"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Priority    int                    `json:"priority"`
	Attempt     int                    `json:"attempt"`
	MaxAttempts int                    `json:"max_attempts"`
	RunAfter    *time.Time             `json:"run_after,omitempty"`
	Error       *string                `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// JobStats holds aggregate counts by status for pipeline jobs.
type JobStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Done      int `json:"done"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

// JobTypeDuration holds per-type timing stats for completed jobs.
type JobTypeDuration struct {
	Type  string  `json:"type"`
	Count int     `json:"count"`
	MinMs int64   `json:"min_ms"`
	MaxMs int64   `json:"max_ms"`
	AvgMs float64 `json:"avg_ms"`
}

// PipelineJobDetail extends PipelineJob with the related file's path.
type PipelineJobDetail struct {
	PipelineJob
	FilePath *string `json:"file_path,omitempty"`
}
