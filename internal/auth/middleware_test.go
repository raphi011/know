package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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
		UserID:      "user123",
		VaultAccess: []string{"vault1", "vault2"},
	}
	ctx = WithAuth(ctx, ac)

	got, err := FromContext(ctx)
	if err != nil {
		t.Fatalf("FromContext failed: %v", err)
	}
	if got.UserID != "user123" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user123")
	}
	if len(got.VaultAccess) != 2 {
		t.Errorf("VaultAccess len = %d, want 2", len(got.VaultAccess))
	}
}

func TestRequireVaultAccess(t *testing.T) {
	ctx := t.Context()
	ac := AuthContext{
		UserID:      "user1",
		VaultAccess: []string{"vault-a", "vault-b"},
	}
	ctx = WithAuth(ctx, ac)

	if err := RequireVaultAccess(ctx, "vault-a"); err != nil {
		t.Errorf("Should have access to vault-a: %v", err)
	}
	if err := RequireVaultAccess(ctx, "vault-c"); err == nil {
		t.Error("Should NOT have access to vault-c")
	}
}

func TestRequireVaultAccess_Wildcard(t *testing.T) {
	ctx := t.Context()
	ac := AuthContext{
		UserID:      "admin",
		VaultAccess: []string{WildcardVaultAccess},
	}
	ctx = WithAuth(ctx, ac)

	if err := RequireVaultAccess(ctx, "any-vault"); err != nil {
		t.Errorf("Wildcard should grant access to any vault: %v", err)
	}
	if err := RequireVaultAccess(ctx, "vault:another"); err != nil {
		t.Errorf("Wildcard should grant access to record-ID vaults: %v", err)
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
		ac := AuthContext{UserID: "user1", VaultAccess: []string{"default"}}

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
		ac := AuthContext{UserID: "user1", VaultAccess: []string{"default"}}

		_, err := ResolveVault(t.Context(), ac, resolver, "secret")
		if err == nil {
			t.Fatal("expected error for no vault access")
		}
	})

	t.Run("vault not found", func(t *testing.T) {
		resolver := &mockVaultResolver{err: fmt.Errorf("not found: %w", os.ErrNotExist)}
		ac := AuthContext{UserID: "user1", VaultAccess: []string{"default"}}

		_, err := ResolveVault(t.Context(), ac, resolver, "missing")
		if err == nil {
			t.Fatal("expected error for missing vault")
		}
	})

	t.Run("nil vault info", func(t *testing.T) {
		resolver := &mockVaultResolver{info: nil, err: nil}
		ac := AuthContext{UserID: "user1", VaultAccess: []string{"default"}}

		_, err := ResolveVault(t.Context(), ac, resolver, "ghost")
		if err == nil {
			t.Fatal("expected error for nil vault info")
		}
	})

	t.Run("wildcard access", func(t *testing.T) {
		resolver := &mockVaultResolver{info: &VaultInfo{ID: "any-vault", Name: "any"}}
		ac := AuthContext{UserID: "admin", VaultAccess: []string{WildcardVaultAccess}}

		id, err := ResolveVault(t.Context(), ac, resolver, "any")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "any-vault" {
			t.Errorf("id = %q, want any-vault", id)
		}
	})
}

func TestCheckVaultAccess(t *testing.T) {
	ac := AuthContext{UserID: "user1", VaultAccess: []string{"vault-a", "vault-b"}}

	if err := CheckVaultAccess(ac, "vault-a"); err != nil {
		t.Errorf("should have access to vault-a: %v", err)
	}
	if err := CheckVaultAccess(ac, "vault-c"); err == nil {
		t.Error("should NOT have access to vault-c")
	}

	admin := AuthContext{UserID: "admin", VaultAccess: []string{WildcardVaultAccess}}
	if err := CheckVaultAccess(admin, "anything"); err != nil {
		t.Errorf("wildcard should grant access: %v", err)
	}
}

func TestMiddlewareMissingHeader(t *testing.T) {
	handler := Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without auth")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}
}
