package auth

import (
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if !strings.HasPrefix(raw, "kh_") {
		t.Errorf("raw token should start with kh_, got %q", raw)
	}

	// kh_ (3 chars) + 64 hex chars = 67
	if len(raw) != 67 {
		t.Errorf("raw token should be 67 chars, got %d", len(raw))
	}

	if len(hash) != 64 {
		t.Errorf("hash should be 64 hex chars, got %d", len(hash))
	}

	// Hash should match what HashToken produces
	if got := HashToken(raw); got != hash {
		t.Errorf("HashToken(raw) = %q, want %q", got, hash)
	}

	// Two calls should produce different tokens
	raw2, _, err := GenerateToken()
	if err != nil {
		t.Fatalf("second GenerateToken() error: %v", err)
	}
	if raw == raw2 {
		t.Error("two generated tokens should not be identical")
	}
}

func TestUseToken(t *testing.T) {
	// Valid token
	hash, err := UseToken("kh_abc123")
	if err != nil {
		t.Fatalf("UseToken() error: %v", err)
	}
	if hash != HashToken("kh_abc123") {
		t.Errorf("UseToken hash mismatch")
	}

	// Invalid prefix
	_, err = UseToken("bad_token")
	if err == nil {
		t.Error("expected error for token without kh_ prefix")
	}

	// Empty string
	_, err = UseToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestHashToken(t *testing.T) {
	h1 := HashToken("kh_test")
	h2 := HashToken("kh_test")
	h3 := HashToken("kh_other")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("SHA256 hex should be 64 chars, got %d", len(h1))
	}
}
