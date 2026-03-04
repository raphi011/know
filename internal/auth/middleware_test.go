package auth

import (
	"net/http"
	"net/http/httptest"
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
