package db

import (
	"context"
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestCreateConversation(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	if conv == nil {
		t.Fatal("CreateConversation returned nil")
	}
	if conv.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestGetConversation(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil for existing conversation")
	}

	// Test nonexistent
	notFound, err := testDB.GetConversation(ctx, "nonexistent-conv-id")
	if err != nil {
		t.Fatalf("GetConversation nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent conversation")
	}
}

func TestListConversations(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	for i := range 2 {
		_, err := testDB.CreateConversation(ctx, vaultID, userID)
		if err != nil {
			t.Fatalf("CreateConversation %d failed: %v", i, err)
		}
	}

	convs, err := testDB.ListConversations(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}
	if len(convs) < 2 {
		t.Errorf("Expected at least 2 conversations, got %d", len(convs))
	}
}

func TestUpdateConversationTitle(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	err = testDB.UpdateConversationTitle(ctx, convID, "New Title")
	if err != nil {
		t.Fatalf("UpdateConversationTitle failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation after update failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil after title update")
	}
	if got.Title != "New Title" {
		t.Errorf("Expected title 'New Title', got %q", got.Title)
	}
}

func TestDeleteConversation(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	err = testDB.DeleteConversation(ctx, convID)
	if err != nil {
		t.Fatalf("DeleteConversation failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation after delete failed: %v", err)
	}
	if got != nil {
		t.Error("Expected nil after deleting conversation")
	}
}

func TestCreateMessage(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	msg, err := testDB.CreateMessage(ctx, convID, models.RoleUser, "Hello", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}
	if msg == nil {
		t.Fatal("CreateMessage returned nil")
	}
	if msg.Role != models.RoleUser {
		t.Errorf("Expected role %q, got %q", models.RoleUser, msg.Role)
	}
	if msg.Content != "Hello" {
		t.Errorf("Expected content 'Hello', got %q", msg.Content)
	}
}

func TestListMessages(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	_, err = testDB.CreateMessage(ctx, convID, models.RoleUser, "Hello", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateMessage 1 failed: %v", err)
	}
	_, err = testDB.CreateMessage(ctx, convID, models.RoleAssistant, "Hi there", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateMessage 2 failed: %v", err)
	}

	msgs, err := testDB.ListMessages(ctx, convID)
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}
}

func TestUpdateConversationTokens(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	err = testDB.UpdateConversationTokens(ctx, convID, 100, 50)
	if err != nil {
		t.Fatalf("UpdateConversationTokens failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.TokenInput != 100 {
		t.Errorf("Expected token_input 100, got %d", got.TokenInput)
	}
	if got.TokenOutput != 50 {
		t.Errorf("Expected token_output 50, got %d", got.TokenOutput)
	}
}

func TestSetConversationBgRunning(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	err = testDB.SetConversationBgRunning(ctx, convID)
	if err != nil {
		t.Fatalf("SetConversationBgRunning failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.BgStatus == nil || *got.BgStatus != "running" {
		t.Errorf("Expected bg_status 'running', got %v", got.BgStatus)
	}
	if got.BgStartedAt == nil {
		t.Error("Expected bg_started_at to be set")
	}
}

func TestSetConversationBgCompleted(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	if err = testDB.SetConversationBgRunning(ctx, convID); err != nil {
		t.Fatalf("SetConversationBgRunning failed: %v", err)
	}

	err = testDB.SetConversationBgCompleted(ctx, convID)
	if err != nil {
		t.Fatalf("SetConversationBgCompleted failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.BgStatus == nil || *got.BgStatus != "completed" {
		t.Errorf("Expected bg_status 'completed', got %v", got.BgStatus)
	}
}

func TestSetConversationBgFailed(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	if err = testDB.SetConversationBgRunning(ctx, convID); err != nil {
		t.Fatalf("SetConversationBgRunning failed: %v", err)
	}

	errMsg := "something went wrong"
	err = testDB.SetConversationBgFailed(ctx, convID, errMsg)
	if err != nil {
		t.Fatalf("SetConversationBgFailed failed: %v", err)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.BgStatus == nil || *got.BgStatus != "failed" {
		t.Errorf("Expected bg_status 'failed', got %v", got.BgStatus)
	}
	if got.BgError == nil || *got.BgError != errMsg {
		t.Errorf("Expected bg_error %q, got %v", errMsg, got.BgError)
	}
}

func TestReconcileStaleRunningConversations(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	conv, err := testDB.CreateConversation(ctx, vaultID, userID)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	convID := models.MustRecordIDString(conv.ID)

	if err = testDB.SetConversationBgRunning(ctx, convID); err != nil {
		t.Fatalf("SetConversationBgRunning failed: %v", err)
	}

	count, err := testDB.ReconcileStaleRunningConversations(ctx)
	if err != nil {
		t.Fatalf("ReconcileStaleRunningConversations failed: %v", err)
	}
	if count < 1 {
		t.Errorf("Expected at least 1 reconciled conversation, got %d", count)
	}

	got, err := testDB.GetConversation(ctx, convID)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.BgStatus == nil || *got.BgStatus != "failed" {
		t.Errorf("Expected bg_status 'failed' after reconcile, got %v", got.BgStatus)
	}
}
