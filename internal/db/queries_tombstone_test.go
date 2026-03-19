package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestListTombstonesSince(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create and then delete a file — SurrealDB event creates a tombstone.
	doc, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/tombstone-" + suffix + "/deleted.md", Title: "Deleted",
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	beforeDelete := time.Now()
	time.Sleep(10 * time.Millisecond)

	if err := testDB.DeleteFile(ctx, docID); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// SurrealDB ASYNC events may take a moment to complete.
	time.Sleep(100 * time.Millisecond)

	// Query tombstones since before the delete.
	tombstones, err := testDB.ListTombstonesSince(ctx, vaultID, beforeDelete)
	if err != nil {
		t.Fatalf("ListTombstonesSince failed: %v", err)
	}

	found := false
	for _, ts := range tombstones {
		if ts.Path == "/tombstone-"+suffix+"/deleted.md" {
			found = true
			if ts.FileID == "" {
				t.Error("tombstone file_id should not be empty")
			}
		}
	}
	if !found {
		t.Errorf("expected tombstone for /tombstone-%s/deleted.md, got %d tombstones", suffix, len(tombstones))
	}

	// Query tombstones since a time well in the future — should return none.
	future := time.Now().Add(time.Hour)
	tombstones, err = testDB.ListTombstonesSince(ctx, vaultID, future)
	if err != nil {
		t.Fatalf("ListTombstonesSince (future) failed: %v", err)
	}
	for _, ts := range tombstones {
		if ts.Path == "/tombstone-"+suffix+"/deleted.md" {
			t.Error("tombstone should not appear when querying after deletion time")
		}
	}
}

func TestPurgeTombstonesBefore(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create and delete two files.
	for i := range 2 {
		doc, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID, Path: fmt.Sprintf("/purge-%s/%d.md", suffix, i), Title: fmt.Sprintf("Purge %d", i),
		})
		if err != nil {
			t.Fatalf("CreateFile %d failed: %v", i, err)
		}
		if err := testDB.DeleteFile(ctx, models.MustRecordIDString(doc.ID)); err != nil {
			t.Fatalf("DeleteFile %d failed: %v", i, err)
		}
	}

	// Wait for ASYNC SurrealDB events.
	time.Sleep(100 * time.Millisecond)

	// Verify tombstones exist.
	tombstones, err := testDB.ListTombstonesSince(ctx, vaultID, time.Time{})
	if err != nil {
		t.Fatalf("ListTombstonesSince failed: %v", err)
	}
	count := 0
	for _, ts := range tombstones {
		if ts.Path == fmt.Sprintf("/purge-%s/0.md", suffix) || ts.Path == fmt.Sprintf("/purge-%s/1.md", suffix) {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("expected at least 2 tombstones, found %d matching", count)
	}

	// Purge tombstones before a future time — should remove them.
	purged, err := testDB.PurgeTombstonesBefore(ctx, vaultID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("PurgeTombstonesBefore failed: %v", err)
	}
	if purged < 2 {
		t.Errorf("expected at least 2 purged, got %d", purged)
	}

	// Verify tombstones are gone.
	tombstones, err = testDB.ListTombstonesSince(ctx, vaultID, time.Time{})
	if err != nil {
		t.Fatalf("ListTombstonesSince after purge failed: %v", err)
	}
	for _, ts := range tombstones {
		if ts.Path == fmt.Sprintf("/purge-%s/0.md", suffix) || ts.Path == fmt.Sprintf("/purge-%s/1.md", suffix) {
			t.Errorf("tombstone %s should have been purged", ts.Path)
		}
	}
}
