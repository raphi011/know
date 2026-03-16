// internal/blob/store.go
package blob

import (
	"context"
	"io"
)

// Store provides content-addressed blob storage.
type Store interface {
	Put(ctx context.Context, hash string, r io.Reader, size int64) error
	Get(ctx context.Context, hash string) (io.ReadCloser, error)
	Exists(ctx context.Context, hash string) (bool, error)
	Delete(ctx context.Context, hash string) error
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
