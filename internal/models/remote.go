package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Remote represents a connection to another know server.
type Remote struct {
	ID        surrealmodels.RecordID `json:"id"`
	Name      string                 `json:"name"`
	URL       string                 `json:"url"`
	Token     string                 `json:"token"`
	CreatedBy surrealmodels.RecordID `json:"created_by"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// RemoteInput holds the data needed to create or update a remote.
type RemoteInput struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}
