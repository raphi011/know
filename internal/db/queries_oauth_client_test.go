package db

import (
	"context"
	"testing"
)

func TestCreateAndGetOAuthClient(t *testing.T) {
	ctx := context.Background()
	clientID := "test_client_" + t.Name()

	err := testDB.CreateOAuthClient(ctx, clientID, "Test App", []string{"http://localhost:1234/callback"})
	if err != nil {
		t.Fatalf("CreateOAuthClient: %v", err)
	}

	client, err := testDB.GetOAuthClient(ctx, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if client == nil {
		t.Fatal("GetOAuthClient returned nil")
	}
	if client.ClientID != clientID {
		t.Errorf("client_id: got %q, want %q", client.ClientID, clientID)
	}
	if client.ClientName != "Test App" {
		t.Errorf("client_name: got %q, want %q", client.ClientName, "Test App")
	}
	if len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "http://localhost:1234/callback" {
		t.Errorf("redirect_uris: got %v, want [http://localhost:1234/callback]", client.RedirectURIs)
	}
	if client.CreatedAt.IsZero() {
		t.Error("created_at should be populated")
	}
}

func TestGetOAuthClientNotFound(t *testing.T) {
	ctx := context.Background()
	client, err := testDB.GetOAuthClient(ctx, "nonexistent_client")
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if client != nil {
		t.Error("expected nil for nonexistent client")
	}
}

func TestCreateOAuthClientDuplicateID(t *testing.T) {
	ctx := context.Background()
	clientID := "test_dup_" + t.Name()

	err := testDB.CreateOAuthClient(ctx, clientID, "App 1", []string{"http://localhost:1111/callback"})
	if err != nil {
		t.Fatalf("CreateOAuthClient (first): %v", err)
	}

	err = testDB.CreateOAuthClient(ctx, clientID, "App 2", []string{"http://localhost:2222/callback"})
	if err == nil {
		t.Fatal("expected error for duplicate client_id")
	}
}
