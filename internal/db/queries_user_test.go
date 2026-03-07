package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateUser(t *testing.T) {
	ctx := context.Background()

	name := fmt.Sprintf("user-%d", time.Now().UnixNano())
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: name})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.Name != name {
		t.Errorf("Expected name %q, got %q", name, user.Name)
	}
}

func TestCreateUserWithID(t *testing.T) {
	ctx := context.Background()

	customID := fmt.Sprintf("custom-user-id-%d", time.Now().UnixNano())
	name := fmt.Sprintf("user-custom-%d", time.Now().UnixNano())
	user, err := testDB.CreateUserWithID(ctx, customID, models.UserInput{Name: name})
	if err != nil {
		t.Fatalf("CreateUserWithID failed: %v", err)
	}

	userID := models.MustRecordIDString(user.ID)
	if userID != customID {
		t.Errorf("Expected user ID %q, got %q", customID, userID)
	}
	if user.Name != name {
		t.Errorf("Expected name %q, got %q", name, user.Name)
	}
}

func TestGetUser(t *testing.T) {
	ctx := context.Background()

	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	retrieved, err := testDB.GetUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetUser returned nil")
	}
	if retrieved.Name != user.Name {
		t.Errorf("Expected name %q, got %q", user.Name, retrieved.Name)
	}

	nonExistent, err := testDB.GetUser(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetUser non-existent should not error: %v", err)
	}
	if nonExistent != nil {
		t.Error("GetUser non-existent should return nil")
	}
}

func TestGetUserByName(t *testing.T) {
	ctx := context.Background()

	uniqueName := fmt.Sprintf("unique-user-%d", time.Now().UnixNano())
	_, err := testDB.CreateUser(ctx, models.UserInput{Name: uniqueName})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	found, err := testDB.GetUserByName(ctx, uniqueName)
	if err != nil {
		t.Fatalf("GetUserByName failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetUserByName returned nil for existing user")
	}
	if found.Name != uniqueName {
		t.Errorf("Expected name %q, got %q", uniqueName, found.Name)
	}

	notFound, err := testDB.GetUserByName(ctx, "nonexistent-user-name")
	if err != nil {
		t.Errorf("GetUserByName non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("GetUserByName non-existent should return nil")
	}
}
