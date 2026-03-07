package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

// TokenInfo holds the validated result of a token lookup.
type TokenInfo struct {
	UserID      string
	VaultAccess []string // bare vault IDs
	TokenID     string   // bare token ID (for last_used updates)
	TokenName   string
}

// ValidateToken hashes the raw token, looks it up in the DB, checks expiry,
// and extracts vault access IDs. Returns an error if the token is invalid or expired.
func ValidateToken(ctx context.Context, dbClient *db.Client, rawToken string) (*TokenInfo, error) {
	hash := HashToken(rawToken)

	token, err := dbClient.GetTokenByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("token lookup: %w", err)
	}
	if token == nil {
		return nil, fmt.Errorf("invalid token")
	}

	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	vaultAccess := make([]string, 0, len(token.VaultAccess))
	for _, v := range token.VaultAccess {
		id, err := models.RecordIDString(v)
		if err != nil {
			continue
		}
		vaultAccess = append(vaultAccess, id)
	}

	if len(vaultAccess) == 0 && len(token.VaultAccess) > 0 {
		return nil, fmt.Errorf("all vault access IDs failed extraction")
	}

	tokenID, _ := models.RecordIDString(token.ID)

	userID, err := models.RecordIDString(token.User)
	if err != nil {
		return nil, fmt.Errorf("extract user ID: %w", err)
	}

	return &TokenInfo{
		UserID:      userID,
		VaultAccess: vaultAccess,
		TokenID:     tokenID,
		TokenName:   token.Name,
	}, nil
}
