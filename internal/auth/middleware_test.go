package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestGenerateToken(t *testing.T) {
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if len(raw) < 10 {
		t.Errorf("Token too short: %q", raw)
	}
	if raw[:3] != "kh_" {
		t.Errorf("Token should start with kh_, got %q", raw[:3])
	}
	if hash == "" {
		t.Error("Hash should not be empty")
	}
	if hash == raw {
		t.Error("Hash should differ from raw token")
	}

	// Same input should produce same hash
	if HashToken(raw) != hash {
		t.Error("HashToken should be deterministic")
	}
}

func TestAuthContextRoundtrip(t *testing.T) {
	ctx := t.Context()

	// No auth context
	_, err := FromContext(ctx)
	if err == nil {
		t.Error("FromContext on empty context should return error")
	}

	// With auth context
	ac := AuthContext{
		UserID:   "user123",
		Provider: ProviderToken,
		Vaults: []models.VaultPermission{
			{VaultID: "vault1", Role: models.RoleRead},
			{VaultID: "vault2", Role: models.RoleWrite},
		},
	}
	ctx = WithAuth(ctx, ac)

	got, err := FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext failed: %v", err)
	}
	if got.UserID != "user123" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user123")
	}
	if got.Provider != ProviderToken {
		t.Errorf("Provider = %q, want %q", got.Provider, ProviderToken)
	}
	if len(got.Vaults) != 2 {
		t.Errorf("Vaults len = %d, want 2", len(got.Vaults))
	}
}

func TestRequireVaultRole(t *testing.T) {
	ctx := t.Context()
	ac := AuthContext{
		UserID: "user1",
		Vaults: []models.VaultPermission{
			{VaultID: "vault-a", Role: models.RoleWrite},
			{VaultID: "vault-b", Role: models.RoleRead},
		},
	}
	ctx = WithAuth(ctx, ac)

	if err := RequireVaultRole(ctx, "vault-a", models.RoleRead); err != nil {
		t.Errorf("Should have read access to vault-a: %v", err)
	}
	if err := RequireVaultRole(ctx, "vault-a", models.RoleWrite); err != nil {
		t.Errorf("Should have write access to vault-a: %v", err)
	}
	if err := RequireVaultRole(ctx, "vault-b", models.RoleWrite); err == nil {
		t.Error("Should NOT have write access to vault-b (only read)")
	}
	if err := RequireVaultRole(ctx, "vault-c", models.RoleRead); err == nil {
		t.Error("Should NOT have access to vault-c")
	}
}

func TestRequireVaultRole_Wildcard(t *testing.T) {
	ctx := t.Context()
	ac := AuthContext{
		UserID:        "admin",
		IsSystemAdmin: true,
		Vaults: []models.VaultPermission{
			{VaultID: WildcardVaultAccess, Role: models.RoleAdmin},
		},
	}
	ctx = WithAuth(ctx, ac)

	if err := RequireVaultRole(ctx, "any-vault", models.RoleAdmin); err != nil {
		t.Errorf("Wildcard should grant admin access to any vault: %v", err)
	}
	if err := RequireVaultRole(ctx, "vault:another", models.RoleWrite); err != nil {
		t.Errorf("Wildcard should grant write access to record-ID vaults: %v", err)
	}
}

// mockVaultResolver implements VaultResolver for tests.
type mockVaultResolver struct {
	info *VaultInfo
	err  error
}

func (m *mockVaultResolver) ResolveByName(_ context.Context, _ string) (*VaultInfo, error) {
	return m.info, m.err
}

func TestResolveVault(t *testing.T) {
	t.Run("vault found with access", func(t *testing.T) {
		resolver := &mockVaultResolver{info: &VaultInfo{ID: "default", Name: "default"}}
		ac := AuthContext{UserID: "user1", Vaults: []models.VaultPermission{
			{VaultID: "default", Role: models.RoleRead},
		}}

		id, err := ResolveVault(t.Context(), ac, resolver, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "default" {
			t.Errorf("id = %q, want default", id)
		}
	})

	t.Run("vault found without access", func(t *testing.T) {
		resolver := &mockVaultResolver{info: &VaultInfo{ID: "secret", Name: "secret"}}
		ac := AuthContext{UserID: "user1", Vaults: []models.VaultPermission{
			{VaultID: "default", Role: models.RoleRead},
		}}

		_, err := ResolveVault(t.Context(), ac, resolver, "secret")
		if err == nil {
			t.Fatal("expected error for no vault access")
		}
	})

	t.Run("vault not found", func(t *testing.T) {
		resolver := &mockVaultResolver{err: fmt.Errorf("not found: %w", os.ErrNotExist)}
		ac := AuthContext{UserID: "user1", Vaults: []models.VaultPermission{
			{VaultID: "default", Role: models.RoleRead},
		}}

		_, err := ResolveVault(t.Context(), ac, resolver, "missing")
		if err == nil {
			t.Fatal("expected error for missing vault")
		}
	})

	t.Run("nil vault info", func(t *testing.T) {
		resolver := &mockVaultResolver{info: nil, err: nil}
		ac := AuthContext{UserID: "user1", Vaults: []models.VaultPermission{
			{VaultID: "default", Role: models.RoleRead},
		}}

		_, err := ResolveVault(t.Context(), ac, resolver, "ghost")
		if err == nil {
			t.Fatal("expected error for nil vault info")
		}
	})

	t.Run("wildcard access", func(t *testing.T) {
		resolver := &mockVaultResolver{info: &VaultInfo{ID: "any-vault", Name: "any"}}
		ac := AuthContext{UserID: "admin", Vaults: []models.VaultPermission{
			{VaultID: WildcardVaultAccess, Role: models.RoleAdmin},
		}}

		id, err := ResolveVault(t.Context(), ac, resolver, "any")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "any-vault" {
			t.Errorf("id = %q, want any-vault", id)
		}
	})
}

func TestCheckVaultRole(t *testing.T) {
	ac := AuthContext{UserID: "user1", Vaults: []models.VaultPermission{
		{VaultID: "vault-a", Role: models.RoleWrite},
		{VaultID: "vault-b", Role: models.RoleRead},
	}}

	if err := CheckVaultRole(ac, "vault-a", models.RoleRead); err != nil {
		t.Errorf("should have read access to vault-a: %v", err)
	}
	if err := CheckVaultRole(ac, "vault-a", models.RoleWrite); err != nil {
		t.Errorf("should have write access to vault-a: %v", err)
	}
	if err := CheckVaultRole(ac, "vault-c", models.RoleRead); err == nil {
		t.Error("should NOT have access to vault-c")
	}

	admin := AuthContext{UserID: "admin", Vaults: []models.VaultPermission{
		{VaultID: WildcardVaultAccess, Role: models.RoleAdmin},
	}}
	if err := CheckVaultRole(admin, "anything", models.RoleAdmin); err != nil {
		t.Errorf("wildcard should grant access: %v", err)
	}
}

func TestMiddlewareMissingHeader(t *testing.T) {
	handler := Middleware(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without auth")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}
}

func TestRequireSystemAdmin(t *testing.T) {
	t.Run("system admin passes", func(t *testing.T) {
		ctx := WithAuth(t.Context(), AuthContext{UserID: "admin", IsSystemAdmin: true})
		if err := RequireSystemAdmin(ctx); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("non-admin fails", func(t *testing.T) {
		ctx := WithAuth(t.Context(), AuthContext{UserID: "user1"})
		if err := RequireSystemAdmin(ctx); err == nil {
			t.Error("expected error for non-admin")
		}
	})

	t.Run("no auth context fails", func(t *testing.T) {
		if err := RequireSystemAdmin(t.Context()); err == nil {
			t.Error("expected error for no auth context")
		}
	})
}

func TestCheckVaultRole_WildcardWithoutSystemAdmin(t *testing.T) {
	// Test wildcard path specifically without IsSystemAdmin
	ac := AuthContext{
		UserID: "user1",
		Vaults: []models.VaultPermission{
			{VaultID: WildcardVaultAccess, Role: models.RoleWrite},
		},
	}

	if err := CheckVaultRole(ac, "any-vault", models.RoleRead); err != nil {
		t.Errorf("wildcard should grant read: %v", err)
	}
	if err := CheckVaultRole(ac, "any-vault", models.RoleWrite); err != nil {
		t.Errorf("wildcard should grant write: %v", err)
	}
	if err := CheckVaultRole(ac, "any-vault", models.RoleAdmin); err == nil {
		t.Error("wildcard with write role should NOT grant admin")
	}
}

func TestVaultRoleMethod(t *testing.T) {
	ac := AuthContext{
		UserID: "user1",
		Vaults: []models.VaultPermission{
			{VaultID: "v1", Role: models.RoleWrite},
			{VaultID: "v2", Role: models.RoleRead},
		},
	}

	if r := ac.VaultRole("v1"); r != models.RoleWrite {
		t.Errorf("VaultRole(v1) = %q, want write", r)
	}
	if r := ac.VaultRole("v2"); r != models.RoleRead {
		t.Errorf("VaultRole(v2) = %q, want read", r)
	}
	if r := ac.VaultRole("v3"); r != "" {
		t.Errorf("VaultRole(v3) = %q, want empty", r)
	}

	// System admin always returns admin
	adminAC := AuthContext{UserID: "admin", IsSystemAdmin: true}
	if r := adminAC.VaultRole("any"); r != models.RoleAdmin {
		t.Errorf("VaultRole for admin = %q, want admin", r)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{
			name:       "no XFF header",
			remoteAddr: "192.168.1.1:1234",
			want:       "192.168.1.1:1234",
		},
		{
			name:       "single IP in XFF",
			xff:        "10.0.0.1",
			remoteAddr: "192.168.1.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "multiple IPs in XFF takes first",
			xff:        "10.0.0.1, 172.16.0.1, 192.168.1.1",
			remoteAddr: "127.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "empty XFF falls back to RemoteAddr",
			xff:        "",
			remoteAddr: "192.168.1.1:1234",
			want:       "192.168.1.1:1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := clientIP(r); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuditEventResult(t *testing.T) {
	tests := []struct {
		event AuditEvent
		want  AuditResult
	}{
		{AuditSuccess, AuditResultOK},
		{AuditFailure, AuditResultDenied},
		{AuditExpired, AuditResultExpired},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			if got := tt.event.Result(); got != tt.want {
				t.Errorf("AuditEvent(%q).Result() = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}
