package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/raphi011/know/internal/models"
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
type vaultIDKey struct{}

// WildcardVaultAccess grants admin access to all vaults (used in no-auth mode).
const WildcardVaultAccess = "*"

// AuthProvider identifies how a user was authenticated.
type AuthProvider string

const (
	// ProviderToken is the default provider for API token authentication.
	ProviderToken AuthProvider = "token"
	// ProviderNoAuth is used in no-auth development mode.
	ProviderNoAuth AuthProvider = "noauth"
	// ProviderOIDC is used for OIDC-authenticated users.
	ProviderOIDC AuthProvider = "oidc"
)

// AuthContext carries authenticated user information through request context.
type AuthContext struct {
	UserID        string
	IsSystemAdmin bool
	Vaults        []models.VaultPermission // vault permissions ("*" with RoleAdmin = all)
	Provider      AuthProvider             // how the user was authenticated
	TokenID       string                   // bare token ID (empty in no-auth mode)
	TokenName     string                   // human-readable token name (empty in no-auth mode)
}

// WithAuth stores auth context in the request context.
func WithAuth(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, contextKey{}, ac)
}

// WithVaultID stores a resolved vault ID in the request context.
// Used by the vault-scoping middleware so handlers can retrieve it via VaultIDFromCtx.
func WithVaultID(ctx context.Context, vaultID string) context.Context {
	return context.WithValue(ctx, vaultIDKey{}, vaultID)
}

// VaultIDFromCtx retrieves the vault ID injected by the vault-scoping middleware.
// Returns an empty string if no vault ID is present.
func VaultIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(vaultIDKey{}).(string)
	return v
}

// MustVaultIDFromCtx retrieves the vault ID injected by the vault-scoping middleware.
// Panics if no vault ID is present — use only in handlers guaranteed to run behind vaultScope.
func MustVaultIDFromCtx(ctx context.Context) string {
	v, ok := ctx.Value(vaultIDKey{}).(string)
	if !ok || v == "" {
		panic("bug: MustVaultIDFromCtx called without vault scope middleware")
	}
	return v
}

// DetachContext creates a new context.Background() with the auth context copied
// from the given request context. This allows background goroutines to retain
// auth info after the original HTTP request context is cancelled.
func DetachContext(requestCtx context.Context) (context.Context, error) {
	ac, err := FromContext(requestCtx)
	if err != nil {
		return nil, err
	}
	return WithAuth(context.Background(), ac), nil
}

// FromContext extracts auth context from the request context.
func FromContext(ctx context.Context) (AuthContext, error) {
	ac, ok := ctx.Value(contextKey{}).(AuthContext)
	if !ok {
		return AuthContext{}, fmt.Errorf("unauthorized: no auth context")
	}
	return ac, nil
}

// RequireVaultRole checks if the authenticated user has at least the given role on a vault.
// Accepts both bare IDs ("default") and record IDs ("vault:default").
func RequireVaultRole(ctx context.Context, vaultID string, minRole models.VaultRole) error {
	ac, err := FromContext(ctx)
	if err != nil {
		return fmt.Errorf("require vault role: %w", err)
	}
	if err := CheckVaultRole(ac, vaultID, minRole); err != nil {
		return fmt.Errorf("require vault role: %w", err)
	}
	return nil
}

// CheckVaultRole checks if an AuthContext has at least the given role on a vault.
func CheckVaultRole(ac AuthContext, vaultID string, minRole models.VaultRole) error {
	if ac.IsSystemAdmin {
		return nil
	}

	bare := models.BareID("vault", vaultID)
	for _, vp := range ac.Vaults {
		if vp.VaultID == WildcardVaultAccess && vp.Role.AtLeast(minRole) {
			return nil
		}
		if vp.VaultID == bare && vp.Role.AtLeast(minRole) {
			return nil
		}
	}
	return fmt.Errorf("forbidden: insufficient role on vault %s", vaultID)
}

// RequireSystemAdmin checks that the authenticated user is a system admin.
func RequireSystemAdmin(ctx context.Context) error {
	ac, err := FromContext(ctx)
	if err != nil {
		return fmt.Errorf("require system admin: %w", err)
	}
	if !ac.IsSystemAdmin {
		return fmt.Errorf("forbidden: system admin required")
	}
	return nil
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

	if err := CheckVaultRole(ac, v.ID, models.RoleRead); err != nil {
		return "", fmt.Errorf("resolve vault: %w", err)
	}

	return v.ID, nil
}

// VaultRole returns the role the user has on the given vault.
// Returns empty string ("") if the user has no access, which has Level() == 0
// and will fail any AtLeast() check.
func (ac AuthContext) VaultRole(vaultID string) models.VaultRole {
	if ac.IsSystemAdmin {
		return models.RoleAdmin
	}
	bare := models.BareID("vault", vaultID)
	for _, vp := range ac.Vaults {
		if vp.VaultID == WildcardVaultAccess || vp.VaultID == bare {
			return vp.Role
		}
	}
	return ""
}
