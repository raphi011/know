package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateRemote(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	name := fmt.Sprintf("remote-%d", time.Now().UnixNano())
	input := models.RemoteInput{
		Name:  name,
		URL:   "https://know.example.com",
		Token: "tok-abc123",
	}

	remote, err := testDB.CreateRemote(ctx, userID, input)
	if err != nil {
		t.Fatalf("CreateRemote failed: %v", err)
	}
	if remote == nil {
		t.Fatal("CreateRemote returned nil")
	}
	if remote.Name != name {
		t.Errorf("expected name %q, got %q", name, remote.Name)
	}
	if remote.URL != input.URL {
		t.Errorf("expected URL %q, got %q", input.URL, remote.URL)
	}
	if remote.Token != input.Token {
		t.Errorf("expected token %q, got %q", input.Token, remote.Token)
	}
	if models.MustRecordIDString(remote.CreatedBy) != userID {
		t.Errorf("expected created_by %q, got %q", userID, models.MustRecordIDString(remote.CreatedBy))
	}

	found, err := testDB.GetRemoteByName(ctx, name)
	if err != nil {
		t.Fatalf("GetRemoteByName failed: %v", err)
	}
	if found == nil {
		t.Fatal("GetRemoteByName returned nil")
	}
	if found.Name != name {
		t.Errorf("round-trip: expected name %q, got %q", name, found.Name)
	}
}

func TestGetRemoteByName_NotFound(t *testing.T) {
	ctx := context.Background()

	found, err := testDB.GetRemoteByName(ctx, "nonexistent-remote-"+fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("GetRemoteByName failed: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil for nonexistent remote, got %+v", found)
	}
}

func TestListRemotes(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	// Use a unique suffix so both names sort together and we can find them.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	nameA := "remote-a-" + suffix
	nameB := "remote-b-" + suffix

	_, err := testDB.CreateRemote(ctx, userID, models.RemoteInput{Name: nameA, URL: "https://a.example.com", Token: "tok-a"})
	if err != nil {
		t.Fatalf("CreateRemote A failed: %v", err)
	}
	_, err = testDB.CreateRemote(ctx, userID, models.RemoteInput{Name: nameB, URL: "https://b.example.com", Token: "tok-b"})
	if err != nil {
		t.Fatalf("CreateRemote B failed: %v", err)
	}

	remotes, err := testDB.ListRemotes(ctx)
	if err != nil {
		t.Fatalf("ListRemotes failed: %v", err)
	}

	names := make(map[string]bool)
	for _, r := range remotes {
		names[r.Name] = true
	}
	if !names[nameA] {
		t.Errorf("expected remote %q in list", nameA)
	}
	if !names[nameB] {
		t.Errorf("expected remote %q in list", nameB)
	}

	// Verify sorted by name ascending (check relative order of our two remotes).
	idxA, idxB := -1, -1
	for i, r := range remotes {
		if r.Name == nameA {
			idxA = i
		}
		if r.Name == nameB {
			idxB = i
		}
	}
	if idxA == -1 || idxB == -1 {
		t.Fatal("could not find both remotes in list")
	}
	if idxA >= idxB {
		t.Errorf("expected %q before %q in sorted list", nameA, nameB)
	}
}

func TestDeleteRemote(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	name := fmt.Sprintf("remote-del-%d", time.Now().UnixNano())
	_, err := testDB.CreateRemote(ctx, userID, models.RemoteInput{Name: name, URL: "https://del.example.com", Token: "tok-del"})
	if err != nil {
		t.Fatalf("CreateRemote failed: %v", err)
	}

	deleted, err := testDB.DeleteRemote(ctx, name)
	if err != nil {
		t.Fatalf("DeleteRemote failed: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteRemote to return true for existing remote")
	}

	found, err := testDB.GetRemoteByName(ctx, name)
	if err != nil {
		t.Fatalf("GetRemoteByName after delete failed: %v", err)
	}
	if found != nil {
		t.Error("expected nil after deletion")
	}

	deletedAgain, err := testDB.DeleteRemote(ctx, name)
	if err != nil {
		t.Fatalf("DeleteRemote nonexistent failed: %v", err)
	}
	if deletedAgain {
		t.Error("expected DeleteRemote to return false for nonexistent remote")
	}
}
