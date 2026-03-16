package blob_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/raphi011/know/internal/blob"
)

func TestFS_PutGet(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)
	ctx := context.Background()

	data := []byte("hello world")
	hash := "abcdef1234567890"

	err := store.Put(ctx, hash, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestFS_PutIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)
	ctx := context.Background()

	data := []byte("idempotent data")
	hash := "aabbccdd11223344"

	err := store.Put(ctx, hash, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}

	err = store.Put(ctx, hash, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
}

func TestFS_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent000000")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestFS_Exists(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)
	ctx := context.Background()

	hash := "eeff00112233aabb"

	exists, err := store.Exists(ctx, hash)
	if err != nil {
		t.Fatalf("Exists before Put: %v", err)
	}
	if exists {
		t.Fatal("expected false before Put")
	}

	data := []byte("exists test")
	err = store.Put(ctx, hash, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	exists, err = store.Exists(ctx, hash)
	if err != nil {
		t.Fatalf("Exists after Put: %v", err)
	}
	if !exists {
		t.Fatal("expected true after Put")
	}
}

func TestFS_Delete(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)
	ctx := context.Background()

	hash := "ddee0011aabbccdd"
	data := []byte("delete me")

	err := store.Put(ctx, hash, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	err = store.Delete(ctx, hash)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Get(ctx, hash)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist after Delete, got %v", err)
	}

	// Delete nonexistent should not error
	err = store.Delete(ctx, "nonexistent000000")
	if err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestFS_LocalPath(t *testing.T) {
	dir := t.TempDir()
	store := blob.NewFS(dir)

	hash := "abcdef1234567890"
	want := filepath.Join(dir, "ab", "cd", hash)
	got := store.LocalPath(hash)

	if got != want {
		t.Fatalf("LocalPath = %q, want %q", got, want)
	}
}

func TestShardedKey(t *testing.T) {
	tests := []struct {
		hash string
		want string
	}{
		{"abcdef1234", "ab/cd/abcdef1234"},
		{"ab", "ab"},
		{"abc", "abc"},
		{"abcd", "ab/cd/abcd"},
	}

	for _, tt := range tests {
		got := blob.ShardedKey(tt.hash)
		if got != tt.want {
			t.Errorf("ShardedKey(%q) = %q, want %q", tt.hash, got, tt.want)
		}
	}
}
