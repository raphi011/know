package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type User struct {
	ID            surrealmodels.RecordID `json:"id"`
	Name          string                 `json:"name"`
	Email         *string                `json:"email,omitempty"`
	IsSystemAdmin bool                   `json:"is_system_admin"`
	CreatedAt     time.Time              `json:"created_at"`
	OIDCProvider  *string                `json:"oidc_provider,omitempty"`
	OIDCSubject   *string                `json:"oidc_subject,omitempty"`
}

type UserInput struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}
