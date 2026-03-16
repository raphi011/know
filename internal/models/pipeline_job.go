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
	UpdatedAt   time.Time              `json:"updated_at"`
}
