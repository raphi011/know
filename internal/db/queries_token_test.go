package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

var testTokenExpiry = time.Now().Add(90 * 24 * time.Hour)

func TestCreateAndLookupToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	token, err := testDB.CreateToken(ctx, userID, "hash123unique"+fmt.Sprint(time.Now().UnixNano()), "test-token", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if token.Name != "test-token" {
		t.Errorf("Expected name 'test-token', got %q", token.Name)
	}
	if token.ExpiresAt == nil {
		t.Fatal("ExpiresAt should be set")
	}
	if token.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}

	found, err := testDB.GetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetTokenByHash failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetTokenByHash returned nil")
	}

	notFound, err := testDB.GetTokenByHash(ctx, "nonexistenthash")
	if err != nil {
		t.Fatalf("GetTokenByHash nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent hash")
	}
}

func TestTokenLastUsed(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	token, err := testDB.CreateToken(ctx, userID, "lastusedhash"+fmt.Sprint(time.Now().UnixNano()), "last-used-token", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	tokenID := models.MustRecordIDString(token.ID)

	if token.LastUsed != nil {
		t.Error("LastUsed should be nil initially")
	}

	err = testDB.UpdateTokenLastUsed(ctx, tokenID)
	if err != nil {
		t.Fatalf("UpdateTokenLastUsed failed: %v", err)
	}

	updated, err := testDB.GetTokenByHash(ctx, token.TokenHash)
	if err != nil {
		t.Fatalf("GetTokenByHash after update failed: %v", err)
	}
	if updated.LastUsed == nil {
		t.Error("LastUsed should be set after update")
	}
}

func TestListTokens(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	for i := range 2 {
		hash := fmt.Sprintf("list-token-hash-%d-%d", i, time.Now().UnixNano())
		name := fmt.Sprintf("list-token-%d", i)
		_, err := testDB.CreateToken(ctx, userID, hash, name, testTokenExpiry)
		if err != nil {
			t.Fatalf("CreateToken %d failed: %v", i, err)
		}
	}

	tokens, err := testDB.ListTokens(ctx, userID)
	if err != nil {
		t.Fatalf("ListTokens failed: %v", err)
	}
	if len(tokens) < 2 {
		t.Errorf("Expected at least 2 tokens, got %d", len(tokens))
	}
}

func TestGetTokenByID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	hash := fmt.Sprintf("get-by-id-hash-%d", time.Now().UnixNano())
	token, err := testDB.CreateToken(ctx, userID, hash, "get-by-id-token", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	tokenID := models.MustRecordIDString(token.ID)

	// Lookup by ID
	found, err := testDB.GetTokenByID(ctx, tokenID)
	if err != nil {
		t.Fatalf("GetTokenByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetTokenByID returned nil")
	}
	if found.Name != "get-by-id-token" {
		t.Errorf("expected name 'get-by-id-token', got %q", found.Name)
	}
	if found.TokenHash != hash {
		t.Errorf("expected hash %q, got %q", hash, found.TokenHash)
	}

	// Lookup with prefixed ID should also work
	foundPrefixed, err := testDB.GetTokenByID(ctx, "api_token:"+tokenID)
	if err != nil {
		t.Fatalf("GetTokenByID with prefix failed: %v", err)
	}
	if foundPrefixed == nil {
		t.Fatal("GetTokenByID with prefix returned nil")
	}

	// Nonexistent ID
	notFound, err := testDB.GetTokenByID(ctx, "nonexistent-token-id")
	if err != nil {
		t.Fatalf("GetTokenByID nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestDeleteExpiredTokens(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// Create an expired token (expires_at in the past)
	expiredHash := fmt.Sprintf("expired-hash-%d", time.Now().UnixNano())
	pastExpiry := time.Now().Add(-1 * time.Hour)
	_, err := testDB.CreateToken(ctx, userID, expiredHash, "expired-token", pastExpiry)
	if err != nil {
		t.Fatalf("CreateToken (expired) failed: %v", err)
	}

	// Create a valid token
	validHash := fmt.Sprintf("valid-hash-%d", time.Now().UnixNano())
	_, err = testDB.CreateToken(ctx, userID, validHash, "valid-token", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken (valid) failed: %v", err)
	}

	// Delete expired tokens
	err = testDB.DeleteExpiredTokens(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredTokens failed: %v", err)
	}

	// Expired token should be gone
	expired, err := testDB.GetTokenByHash(ctx, expiredHash)
	if err != nil {
		t.Fatalf("GetTokenByHash (expired) failed: %v", err)
	}
	if expired != nil {
		t.Error("expired token should have been deleted")
	}

	// Valid token should still exist
	valid, err := testDB.GetTokenByHash(ctx, validHash)
	if err != nil {
		t.Fatalf("GetTokenByHash (valid) failed: %v", err)
	}
	if valid == nil {
		t.Error("valid token should still exist")
	}
}

func TestCountTokens(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// Initially 0
	count, err := testDB.CountTokens(ctx, userID)
	if err != nil {
		t.Fatalf("CountTokens failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens, got %d", count)
	}

	// Create 3 tokens
	for i := range 3 {
		hash := fmt.Sprintf("count-hash-%d-%d", i, time.Now().UnixNano())
		_, err := testDB.CreateToken(ctx, userID, hash, fmt.Sprintf("count-token-%d", i), testTokenExpiry)
		if err != nil {
			t.Fatalf("CreateToken %d failed: %v", i, err)
		}
	}

	count, err = testDB.CountTokens(ctx, userID)
	if err != nil {
		t.Fatalf("CountTokens failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 tokens, got %d", count)
	}
}

func TestRotateToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// Create original token
	oldHash := fmt.Sprintf("rotate-old-hash-%d", time.Now().UnixNano())
	oldToken, err := testDB.CreateToken(ctx, userID, oldHash, "rotate-me", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	oldTokenID := models.MustRecordIDString(oldToken.ID)

	// Rotate
	newHash := fmt.Sprintf("rotate-new-hash-%d", time.Now().UnixNano())
	newExpiry := time.Now().Add(30 * 24 * time.Hour)
	newToken, err := testDB.RotateToken(ctx, oldTokenID, userID, newHash, "rotate-me", newExpiry)
	if err != nil {
		t.Fatalf("RotateToken failed: %v", err)
	}
	if newToken == nil {
		t.Fatal("RotateToken returned nil")
	}
	if newToken.Name != "rotate-me" {
		t.Errorf("expected name 'rotate-me', got %q", newToken.Name)
	}
	if newToken.TokenHash != newHash {
		t.Errorf("expected new hash, got %q", newToken.TokenHash)
	}

	// Old token should be gone
	old, err := testDB.GetTokenByHash(ctx, oldHash)
	if err != nil {
		t.Fatalf("GetTokenByHash (old) failed: %v", err)
	}
	if old != nil {
		t.Error("old token should have been deleted")
	}

	// New token should exist
	found, err := testDB.GetTokenByHash(ctx, newHash)
	if err != nil {
		t.Fatalf("GetTokenByHash (new) failed: %v", err)
	}
	if found == nil {
		t.Error("new token should exist")
	}
}

func TestDeleteToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	hash := fmt.Sprintf("delete-token-hash-%d", time.Now().UnixNano())
	token, err := testDB.CreateToken(ctx, userID, hash, "delete-me-token", testTokenExpiry)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	tokenID := models.MustRecordIDString(token.ID)

	err = testDB.DeleteToken(ctx, tokenID)
	if err != nil {
		t.Fatalf("DeleteToken failed: %v", err)
	}

	deleted, err := testDB.GetTokenByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetTokenByHash after delete failed: %v", err)
	}
	if deleted != nil {
		t.Error("Token should be nil after delete")
	}
}
