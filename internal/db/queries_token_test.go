package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateAndLookupToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	token, err := testDB.CreateToken(ctx, userID, "hash123unique"+fmt.Sprint(time.Now().UnixNano()), "test-token")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if token.Name != "test-token" {
		t.Errorf("Expected name 'test-token', got %q", token.Name)
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

	token, err := testDB.CreateToken(ctx, userID, "lastusedhash"+fmt.Sprint(time.Now().UnixNano()), "last-used-token")
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

	for i := 0; i < 2; i++ {
		hash := fmt.Sprintf("list-token-hash-%d-%d", i, time.Now().UnixNano())
		name := fmt.Sprintf("list-token-%d", i)
		_, err := testDB.CreateToken(ctx, userID, hash, name)
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

func TestDeleteToken(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	hash := fmt.Sprintf("delete-token-hash-%d", time.Now().UnixNano())
	token, err := testDB.CreateToken(ctx, userID, hash, "delete-me-token")
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
