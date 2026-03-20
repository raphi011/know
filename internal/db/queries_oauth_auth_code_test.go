package db

import (
	"context"
	"testing"
	"time"
)

func TestCreateAndGetOAuthAuthCode(t *testing.T) {
	ctx := context.Background()
	code := "oauthcode_test_create_" + t.Name()
	err := testDB.CreateOAuthAuthCode(ctx, code, "encrypted_token_123", "challenge_abc", "http://localhost:1234/callback", time.Now().Add(60*time.Second))
	if err != nil {
		t.Fatalf("CreateOAuthAuthCode: %v", err)
	}
	ac, err := testDB.GetOAuthAuthCode(ctx, code)
	if err != nil {
		t.Fatalf("GetOAuthAuthCode: %v", err)
	}
	if ac == nil {
		t.Fatal("GetOAuthAuthCode returned nil")
	}
	if ac.Code != code {
		t.Errorf("code: got %q, want %q", ac.Code, code)
	}
	if ac.EncryptedToken != "encrypted_token_123" {
		t.Errorf("encrypted_token: got %q, want %q", ac.EncryptedToken, "encrypted_token_123")
	}
	if ac.CodeChallenge != "challenge_abc" {
		t.Errorf("code_challenge: got %q, want %q", ac.CodeChallenge, "challenge_abc")
	}
	if ac.RedirectURI != "http://localhost:1234/callback" {
		t.Errorf("redirect_uri: got %q, want %q", ac.RedirectURI, "http://localhost:1234/callback")
	}
	notFound, err := testDB.GetOAuthAuthCode(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetOAuthAuthCode nonexistent: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for nonexistent code")
	}
}

func TestDeleteOAuthAuthCode(t *testing.T) {
	ctx := context.Background()
	code := "oauthcode_test_delete_" + t.Name()
	err := testDB.CreateOAuthAuthCode(ctx, code, "tok", "chal", "http://localhost:5678/callback", time.Now().Add(60*time.Second))
	if err != nil {
		t.Fatalf("CreateOAuthAuthCode: %v", err)
	}
	err = testDB.DeleteOAuthAuthCode(ctx, code)
	if err != nil {
		t.Fatalf("DeleteOAuthAuthCode: %v", err)
	}
	ac, err := testDB.GetOAuthAuthCode(ctx, code)
	if err != nil {
		t.Fatalf("GetOAuthAuthCode after delete: %v", err)
	}
	if ac != nil {
		t.Error("expected nil after delete")
	}
}

func TestDeleteExpiredOAuthAuthCodes(t *testing.T) {
	ctx := context.Background()
	expiredCode := "oauthcode_test_expired_" + t.Name()
	err := testDB.CreateOAuthAuthCode(ctx, expiredCode, "tok", "chal", "http://localhost:1111/callback", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("CreateOAuthAuthCode (expired): %v", err)
	}
	validCode := "oauthcode_test_valid_" + t.Name()
	err = testDB.CreateOAuthAuthCode(ctx, validCode, "tok2", "chal2", "http://localhost:2222/callback", time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("CreateOAuthAuthCode (valid): %v", err)
	}
	err = testDB.DeleteExpiredOAuthAuthCodes(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredOAuthAuthCodes: %v", err)
	}
	ac, _ := testDB.GetOAuthAuthCode(ctx, expiredCode)
	if ac != nil {
		t.Error("expired auth code should be deleted")
	}
	ac2, _ := testDB.GetOAuthAuthCode(ctx, validCode)
	if ac2 == nil {
		t.Error("valid auth code should still exist")
	}
}
