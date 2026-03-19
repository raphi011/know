package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// DeviceCode represents an OAuth 2.0 Device Authorization Grant request.
type DeviceCode struct {
	ID         surrealmodels.RecordID  `json:"id"`
	UserCode   string                  `json:"user_code"`
	DeviceCode string                  `json:"device_code"`
	ExpiresAt  time.Time               `json:"expires_at"`
	User       *surrealmodels.RecordID `json:"user,omitempty"`
	Approved   bool                    `json:"approved"`
	// TokenHash stores the raw API token temporarily (despite the field name).
	// The device code record is short-lived (15 min max) and deleted after
	// the CLI polls successfully. The raw token is also stored hashed in api_token.
	TokenHash *string   `json:"token_hash,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
