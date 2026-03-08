package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

// Authenticate validates a raw API token and returns an AuthContext.
// If noAuth is true, returns an admin context with wildcard vault access.
// Updates the token's last_used timestamp asynchronously (failures are logged
// but not propagated).
func Authenticate(ctx context.Context, dbClient *db.Client, rawToken string, noAuth bool) (AuthContext, error) {
	if noAuth {
		return AuthContext{
			UserID:      "admin",
			VaultAccess: []string{WildcardVaultAccess},
		}, nil
	}

	info, err := ValidateToken(ctx, dbClient, rawToken)
	if err != nil {
		return AuthContext{}, err
	}

	// Update last_used (fire and forget)
	if info.TokenID != "" {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := dbClient.UpdateTokenLastUsed(bgCtx, info.TokenID); err != nil {
				slog.Warn("failed to update token last_used", "token_id", info.TokenID, "error", err)
			}
		}()
	}

	return AuthContext{
		UserID:      info.UserID,
		VaultAccess: info.VaultAccess,
	}, nil
}

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
			slog.Warn("token vault access ID extraction failed", "raw_id", v, "error", err)
			continue
		}
		vaultAccess = append(vaultAccess, id)
	}

	if len(vaultAccess) == 0 && len(token.VaultAccess) > 0 {
		return nil, fmt.Errorf("all vault access IDs failed extraction")
	}

	tokenID, err := models.RecordIDString(token.ID)
	if err != nil {
		slog.Warn("failed to extract token ID for last_used tracking", "error", err)
		// Non-fatal: authentication succeeds, but last_used won't update
	}

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
