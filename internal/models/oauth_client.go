package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// OAuthClient represents a dynamically registered OAuth client (RFC 7591).
type OAuthClient struct {
	ID           surrealmodels.RecordID `json:"id"`
	ClientID     string                 `json:"client_id"`
	ClientName   string                 `json:"client_name"`
	RedirectURIs []string               `json:"redirect_uris"`
	CreatedAt    time.Time              `json:"created_at"`
}
