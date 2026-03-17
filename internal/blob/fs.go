package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/raphi011/know/internal/logutil"
)

// Compile-time check that FS implements LocalPathStore.
var _ LocalPathStore = (*FS)(nil)

// FS is a content-addressed blob store backed by the local filesystem.
// Blobs are stored in a 2-level sharded directory structure.
type FS struct {
	dir string
}

// NewFS creates a new filesystem-backed blob store rooted at dir.
func NewFS(dir string) *FS {
	return &FS{dir: dir}
}

// LocalPath returns the absolute filesystem path for the given hash.
func (f *FS) LocalPath(hash string) string {
	return filepath.Join(f.dir, ShardedKey(hash))
}

// Put writes the contents of r to the blob store under the given hash.
// If a blob with that hash already exists, Put is a no-op (idempotent).
// The write is atomic: data is written to a temp file then renamed.
func (f *FS) Put(_ context.Context, hash string, r io.Reader, _ int64) error {
	path := f.LocalPath(hash)

	exists, err := f.exists(path)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}
	if exists {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("put: mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return fmt.Errorf("put: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("put: write: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("put: close: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("put: rename: %w", err)
	}

	return nil
}

// PutVerified streams r to a temp file while computing its SHA256 hash.
// If the computed hash matches expectedHash, the temp file is atomically renamed
// to the final path. If it doesn't match, the temp file is deleted and a
// *HashMismatchError is returned. This ensures the content-addressed store is
// never corrupted by incorrect data — the final path is never written with
// unverified content.
func (f *FS) PutVerified(ctx context.Context, expectedHash string, r io.Reader, _ int64) error {
	path := f.LocalPath(expectedHash)

	exists, err := f.exists(path)
	if err != nil {
		return fmt.Errorf("put verified: %w", err)
	}
	if exists {
		// Drain the reader so callers using streaming readers (e.g. multipart
		// parts) don't have unconsumed bytes corrupt subsequent reads.
		if _, err := io.Copy(io.Discard, r); err != nil {
			return fmt.Errorf("put verified: drain existing: %w", err)
		}
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("put verified: mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return fmt.Errorf("put verified: create temp: %w", err)
	}
	tmpName := tmp.Name()

	// TeeReader: every byte read from r is also written to the hasher.
	hasher := sha256.New()
	tee := io.TeeReader(r, hasher)

	if _, err := io.Copy(tmp, tee); err != nil {
		tmp.Close()
		f.removeTmp(ctx, tmpName)
		return fmt.Errorf("put verified: write: %w", err)
	}

	if err := tmp.Close(); err != nil {
		f.removeTmp(ctx, tmpName)
		return fmt.Errorf("put verified: close: %w", err)
	}

	// Verify hash before committing. If mismatch, delete temp — final path is never touched.
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		f.removeTmp(ctx, tmpName)
		return &HashMismatchError{Expected: expectedHash, Actual: actualHash}
	}

	if err := os.Rename(tmpName, path); err != nil {
		f.removeTmp(ctx, tmpName)
		return fmt.Errorf("put verified: rename: %w", err)
	}

	return nil
}

// Get opens the blob identified by hash for reading.
// Returns os.ErrNotExist if the blob does not exist.
func (f *FS) Get(_ context.Context, hash string) (io.ReadCloser, error) {
	file, err := os.Open(f.LocalPath(hash))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("get %s: %w", hash, os.ErrNotExist)
		}
		return nil, fmt.Errorf("get: %w", err)
	}
	return file, nil
}

// Exists reports whether a blob with the given hash exists.
func (f *FS) Exists(_ context.Context, hash string) (bool, error) {
	return f.exists(f.LocalPath(hash))
}

// Delete removes the blob identified by hash.
// If the blob does not exist, Delete is a no-op.
func (f *FS) Delete(_ context.Context, hash string) error {
	err := os.Remove(f.LocalPath(hash))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// removeTmp attempts to remove a temp file and logs a warning on failure.
func (f *FS) removeTmp(ctx context.Context, path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		logutil.FromCtx(ctx).Warn("failed to remove temp blob file", "path", path, "error", err)
	}
}

func (f *FS) exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
