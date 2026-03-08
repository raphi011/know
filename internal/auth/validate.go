package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

// Authenticate validates a raw API token and returns an AuthContext.
// If noAuth is true, returns a system admin context with wildcard vault access.
// Falls back to share link lookup if token is not found in api_token table.
func Authenticate(ctx context.Context, dbClient *db.Client, rawToken string, noAuth bool) (AuthContext, error) {
	if noAuth {
		return AuthContext{
			UserID:        "admin",
			IsSystemAdmin: true,
			Vaults:        []models.VaultPermission{{VaultID: WildcardVaultAccess, Role: models.RoleAdmin}},
		}, nil
	}

	// Try API token first
	info, err := ValidateToken(ctx, dbClient, rawToken)
	if err == nil {
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
			UserID:        info.UserID,
			IsSystemAdmin: info.IsSystemAdmin,
			Vaults:        info.Vaults,
		}, nil
	}

	// Only fall through to share link if the token was genuinely not found.
	// DB errors (connectivity, timeouts) should propagate immediately.
	if !isNotFoundError(err) {
		return AuthContext{}, err
	}

	// Try share link fallback
	shareCtx, shareErr := ValidateShareLink(ctx, dbClient, rawToken)
	if shareErr == nil {
		return AuthContext{
			Share: shareCtx,
			Vaults: []models.VaultPermission{{
				VaultID: shareCtx.VaultID,
				Role:    models.RoleRead,
			}},
		}, nil
	}

	// Return the original token error (more informative than share link error)
	return AuthContext{}, err
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
		return nil, fmt.Errorf("invalid token")
	}

	if token.IsExpired() {
		return nil, fmt.Errorf("token expired")
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

// ValidateShareLink checks if the raw token corresponds to a valid share link.
func ValidateShareLink(ctx context.Context, dbClient *db.Client, rawToken string) (*ShareContext, error) {
	hash := HashToken(rawToken)

	link, err := dbClient.GetShareLinkByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("share link lookup: %w", err)
	}
	if link == nil {
		return nil, fmt.Errorf("invalid share link")
	}

	if link.IsExpired() {
		return nil, fmt.Errorf("share link expired")
	}

	vaultID, err := models.RecordIDString(link.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault ID from share link: %w", err)
	}

	return &ShareContext{
		VaultID:  vaultID,
		Path:     link.Path,
		IsFolder: link.IsFolder,
	}, nil
}

// isNotFoundError returns true if the error indicates a token/link was not found
// (as opposed to a DB infrastructure error).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid token") || strings.Contains(msg, "token expired")
}
