package db

import (
	"context"
	"testing"
)

func TestCheckpoint_GetNonExistent(t *testing.T) {
	ctx := context.Background()
	data, err := testDB.GetCheckpoint(ctx, "nonexistent-checkpoint")
	if err != nil {
		t.Fatalf("GetCheckpoint error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for nonexistent checkpoint, got %d bytes", len(data))
	}
}

func TestCheckpoint_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	id := "test-checkpoint-roundtrip"
	payload := []byte(`{"state":"interrupted","tool":"create_document"}`)

	if err := testDB.UpsertCheckpoint(ctx, id, payload); err != nil {
		t.Fatalf("UpsertCheckpoint failed: %v", err)
	}

	got, err := testDB.GetCheckpoint(ctx, id)
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetCheckpoint returned nil after upsert")
	}
	if string(got) != string(payload) {
		t.Errorf("data mismatch: got %q, want %q", got, payload)
	}
}

func TestCheckpoint_UpsertOverwrites(t *testing.T) {
	ctx := context.Background()
	id := "test-checkpoint-overwrite"

	if err := testDB.UpsertCheckpoint(ctx, id, []byte("first")); err != nil {
		t.Fatalf("first UpsertCheckpoint failed: %v", err)
	}

	if err := testDB.UpsertCheckpoint(ctx, id, []byte("second")); err != nil {
		t.Fatalf("second UpsertCheckpoint failed: %v", err)
	}

	got, err := testDB.GetCheckpoint(ctx, id)
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("expected overwritten data %q, got %q", "second", got)
	}
}

func TestCheckpoint_Delete(t *testing.T) {
	ctx := context.Background()
	id := "test-checkpoint-delete"

	if err := testDB.UpsertCheckpoint(ctx, id, []byte("to-delete")); err != nil {
		t.Fatalf("UpsertCheckpoint failed: %v", err)
	}

	if err := testDB.DeleteCheckpoint(ctx, id); err != nil {
		t.Fatalf("DeleteCheckpoint failed: %v", err)
	}

	got, err := testDB.GetCheckpoint(ctx, id)
	if err != nil {
		t.Fatalf("GetCheckpoint after delete failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %d bytes", len(got))
	}
}

func TestCheckpoint_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	// Deleting a non-existent checkpoint should not error.
	if err := testDB.DeleteCheckpoint(ctx, "never-existed"); err != nil {
		t.Fatalf("DeleteCheckpoint non-existent error: %v", err)
	}
}

func TestCheckpoint_EmptyPayload(t *testing.T) {
	ctx := context.Background()
	id := "test-checkpoint-empty"

	if err := testDB.UpsertCheckpoint(ctx, id, []byte{}); err != nil {
		t.Fatalf("UpsertCheckpoint empty failed: %v", err)
	}

	got, err := testDB.GetCheckpoint(ctx, id)
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil data for empty payload")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 bytes, got %d", len(got))
	}
}
