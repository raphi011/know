package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Conversation represents a persistent chat session.
type Conversation struct {
	ID        surrealmodels.RecordID `json:"id"`
	Title     string                 `json:"title"`
	EntityID  *string                `json:"entity_id,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Message represents a single chat message within a conversation.
type Message struct {
	ID           surrealmodels.RecordID `json:"id"`
	Conversation surrealmodels.RecordID `json:"conversation"`
	Role         string                 `json:"role"`
	Content      string                 `json:"content"`
	CreatedAt    time.Time              `json:"created_at"`
}
