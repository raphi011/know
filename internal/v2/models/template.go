package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Template struct {
	ID           surrealmodels.RecordID  `json:"id"`
	Vault        *surrealmodels.RecordID `json:"vault,omitempty"`
	Name         string                  `json:"name"`
	Description  *string                 `json:"description,omitempty"`
	Content      string                  `json:"content"`
	IsAITemplate bool                    `json:"is_ai_template"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

type TemplateInput struct {
	VaultID      *string `json:"vault_id,omitempty"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	Content      string  `json:"content"`
	IsAITemplate bool    `json:"is_ai_template"`
}
