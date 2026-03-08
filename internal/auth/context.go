package auth

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/raphi011/knowhow/internal/models"
)

// VaultInfo is the minimal vault data needed for auth resolution.
type VaultInfo struct {
	ID   string // bare vault ID (e.g. "default")
	Name string
}

// VaultResolver resolves vaults by name for auth checks.
type VaultResolver interface {
	ResolveByName(ctx context.Context, name string) (*VaultInfo, error)
}

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

// RequireVaultAccess checks if the authenticated user (from context) has access to a vault.
// Accepts both bare IDs ("default") and record IDs ("vault:default").
// Wildcard access ("*") grants access to all vaults (no-auth mode).
func RequireVaultAccess(ctx context.Context, vaultID string) error {
	ac, err := FromContext(ctx)
	if err != nil {
		return err
	}
	return CheckVaultAccess(ac, vaultID)
}

// CheckVaultAccess checks if an AuthContext has access to the given vault ID.
// Accepts both bare IDs ("default") and record IDs ("vault:default").
func CheckVaultAccess(ac AuthContext, vaultID string) error {
	if slices.Contains(ac.VaultAccess, WildcardVaultAccess) {
		return nil
	}
	bare := models.BareID("vault", vaultID)
	if slices.Contains(ac.VaultAccess, bare) {
		return nil
	}
	return fmt.Errorf("forbidden: no access to vault %s", vaultID)
}

// ResolveVault looks up a vault by name and checks if the given AuthContext has access.
// Returns the bare vault ID on success.
func ResolveVault(ctx context.Context, ac AuthContext, vaultSvc VaultResolver, vaultName string) (string, error) {
	v, err := vaultSvc.ResolveByName(ctx, vaultName)
	if err != nil {
		return "", fmt.Errorf("vault lookup %q: %w", vaultName, err)
	}
	if v == nil {
		return "", fmt.Errorf("vault %q not found: %w", vaultName, os.ErrNotExist)
	}

	if err := CheckVaultAccess(ac, v.ID); err != nil {
		return "", err
	}

	return v.ID, nil
}
