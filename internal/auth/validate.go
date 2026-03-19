package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
)

var (
	// ErrInvalidToken indicates the token was not found in the database.
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired indicates the token was found but has expired.
	ErrTokenExpired = errors.New("token expired")
)

// Authenticate validates a raw API token and returns an AuthContext.
// If noAuth is true, returns a system admin context with wildcard vault access.
func Authenticate(ctx context.Context, dbClient *db.Client, rawToken string, noAuth bool) (AuthContext, error) {
	if noAuth {
		return AuthContext{
			UserID:        "admin",
			IsSystemAdmin: true,
			Vaults:        []models.VaultPermission{{VaultID: WildcardVaultAccess, Role: models.RoleAdmin}},
			Provider:      ProviderNoAuth,
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
				slog.Warn("failed to update token last_used", "token_id", info.TokenID, "user_id", info.UserID, "error", err)
			}
		}()
	}

	return AuthContext{
		UserID:        info.UserID,
		IsSystemAdmin: info.IsSystemAdmin,
		Vaults:        info.Vaults,
		Provider:      ProviderToken,
	}, nil
}

// TokenInfo holds the validated result of a token lookup.
type TokenInfo struct {
	UserID        string
	IsSystemAdmin bool
	Vaults        []models.VaultPermission
	TokenID       string // bare token ID (for last_used updates)
	TokenName     string
}

// ValidateToken hashes the raw token, looks it up in the DB, checks expiry,
// and queries vault memberships. Returns an error if the token is invalid or expired.
func ValidateToken(ctx context.Context, dbClient *db.Client, rawToken string) (*TokenInfo, error) {
	hash := HashToken(rawToken)

	token, err := dbClient.GetTokenByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("token lookup: %w", err)
	}
	if token == nil {
		return nil, ErrInvalidToken
	}

	if token.IsExpired() {
		return nil, ErrTokenExpired
	}

	tokenID, err := models.RecordIDString(token.ID)
	if err != nil {
		// Non-fatal: authentication succeeds, but last_used won't update
		slog.Warn("failed to extract token ID for last_used tracking", "error", err)
	}

	userID, err := models.RecordIDString(token.User)
	if err != nil {
		return nil, fmt.Errorf("extract user ID: %w", err)
	}

	// Get user for is_system_admin
	user, err := dbClient.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found for token")
	}

	// Query vault memberships
	memberships, err := dbClient.GetVaultMemberships(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get vault memberships: %w", err)
	}

	vaults := make([]models.VaultPermission, 0, len(memberships))
	for _, m := range memberships {
		vid, err := models.RecordIDString(m.Vault)
		if err != nil {
			slog.Warn("vault member vault ID extraction failed", "error", err)
			continue
		}
		role, err := models.ParseVaultRole(m.Role)
		if err != nil {
			slog.Warn("vault member has invalid role", "vault", vid, "role", m.Role, "error", err)
			continue
		}
		vaults = append(vaults, models.VaultPermission{
			VaultID: vid,
			Role:    role,
		})
	}

	// Safety net: if the user has memberships but all failed to parse,
	// something is wrong — don't silently grant zero access.
	if len(memberships) > 0 && len(vaults) == 0 {
		return nil, fmt.Errorf("failed to resolve any vault memberships for user %s", userID)
	}

	return &TokenInfo{
		UserID:        userID,
		IsSystemAdmin: user.IsSystemAdmin,
		Vaults:        vaults,
		TokenID:       tokenID,
		TokenName:     token.Name,
	}, nil
}
