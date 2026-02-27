package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Vault struct {
	ID          surrealmodels.RecordID `json:"id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	CreatedBy   surrealmodels.RecordID `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type VaultInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}
