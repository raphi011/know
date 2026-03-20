package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// OAuthAuthCode represents a short-lived authorization code issued during the
// OAuth flow. Maps code → encrypted kh_ token + PKCE challenge + redirect_uri.
type OAuthAuthCode struct {
	ID             surrealmodels.RecordID `json:"id"`
	Code           string                 `json:"code"`
	EncryptedToken string                 `json:"encrypted_token"`
	CodeChallenge  string                 `json:"code_challenge"`
	RedirectURI    string                 `json:"redirect_uri"`
	ExpiresAt      time.Time              `json:"expires_at"`
	CreatedAt      time.Time              `json:"created_at"`
}
