// internal/blob/store.go
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Store provides content-addressed blob storage.
type Store interface {
	Put(ctx context.Context, hash string, r io.Reader, size int64) error
	// PutVerified streams r to storage while computing its SHA256 hash.
	// The blob is only committed if the computed hash matches expectedHash.
	// On mismatch, no data is persisted and a *HashMismatchError is returned.
	PutVerified(ctx context.Context, expectedHash string, r io.Reader, size int64) error
	Get(ctx context.Context, hash string) (io.ReadCloser, error)
	Exists(ctx context.Context, hash string) (bool, error)
	Delete(ctx context.Context, hash string) error
}

// HashMismatchError is returned by PutVerified when the computed hash does not
// match the expected hash. This indicates the client sent corrupt or incorrect data.
type HashMismatchError struct {
	Expected string
	Actual   string
}

func (e *HashMismatchError) Error() string {
	return fmt.Sprintf("hash mismatch: expected %s, got %s", e.Expected, e.Actual)
}

// IsHashMismatch reports whether err (or any wrapped error) is a *HashMismatchError.
func IsHashMismatch(err error) bool {
	var e *HashMismatchError
	return errors.As(err, &e)
}

// LocalPathStore is optionally implemented by backends that store blobs
// on the local filesystem. Allows direct file access (e.g., for ffmpeg).
type LocalPathStore interface {
	Store
	LocalPath(hash string) string
}

// ShardedKey returns the 2-level sharded path for a hash: ab/cd/abcdef...
func ShardedKey(hash string) string {
	if len(hash) < 4 {
		return hash
	}
	return hash[:2] + "/" + hash[2:4] + "/" + hash
}
