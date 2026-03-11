package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Label represents a label record backed by the label table.
type Label struct {
	ID        surrealmodels.RecordID `json:"id"`
	Name      string                 `json:"name"`
	Vault     surrealmodels.RecordID `json:"vault"`
	CreatedAt time.Time              `json:"created_at"`
}
