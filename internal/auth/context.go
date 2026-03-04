package auth

import (
	"context"
	"fmt"
	"slices"

	"github.com/raphi011/knowhow/internal/models"
)

type contextKey struct{}

// WildcardVaultAccess grants access to all vaults (used in no-auth mode).
const WildcardVaultAccess = "*"

// AuthContext carries authenticated user information through request context.
type AuthContext struct {
	UserID      string
	VaultAccess []string // vault IDs the user can access ("*" = all)
}

// WithAuth stores auth context in the request context.
func WithAuth(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, contextKey{}, ac)
}

// FromContext extracts auth context from the request context.
func FromContext(ctx context.Context) (AuthContext, error) {
	ac, ok := ctx.Value(contextKey{}).(AuthContext)
	if !ok {
		return AuthContext{}, fmt.Errorf("unauthorized: no auth context")
	}
	return ac, nil
}

// RequireVaultAccess checks if the authenticated user has access to a vault.
// Accepts both bare IDs ("default") and record IDs ("vault:default").
// Wildcard access ("*") grants access to all vaults (no-auth mode).
func RequireVaultAccess(ctx context.Context, vaultID string) error {
	ac, err := FromContext(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(ac.VaultAccess, WildcardVaultAccess) {
		return nil
	}
	bare := models.BareID("vault", vaultID)
	if slices.Contains(ac.VaultAccess, bare) {
		return nil
	}
	return fmt.Errorf("forbidden: no access to vault %s", vaultID)
}
