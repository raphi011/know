package blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Compile-time check that S3 implements Store.
var _ Store = (*S3)(nil)

// S3 is a content-addressed blob store backed by an S3-compatible service.
type S3 struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3 creates a new S3-backed blob store.
// Prefix is prepended to all object keys (e.g. "blobs").
func NewS3(client *s3.Client, bucket, prefix string) *S3 {
	return &S3{client: client, bucket: bucket, prefix: prefix}
}

// key returns the full S3 object key for a content hash.
func (s *S3) key(hash string) string {
	return s.prefix + "/" + ShardedKey(hash)
}

// Put uploads the contents of r to S3 under the given hash.
// S3 PutObject is idempotent for content-addressed storage (same hash = same content),
// so no existence check is needed.
func (s *S3) Put(ctx context.Context, hash string, r io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           new(s.key(hash)),
		Body:          r,
		ContentLength: &size,
	})
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	return nil
}

// PutVerified uploads data to S3 using multipart upload with hash verification.
// The upload is only completed if the computed SHA256 matches expectedHash.
// On mismatch, the multipart upload is aborted and no object is created,
// ensuring the content-addressed store is never corrupted.
//
// For simplicity, this implementation buffers the data into memory to compute
// the hash before uploading. For very large files, a streaming multipart upload
// with per-part hashing would be more memory-efficient, but the current approach
// is sufficient for the expected file sizes (images, audio < 100 MB).
func (s *S3) PutVerified(ctx context.Context, expectedHash string, r io.Reader, size int64) error {
	// Read all data to compute hash before uploading.
	// This ensures we never upload data under the wrong key.
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("put verified: read: %w", err)
	}

	h := sha256.Sum256(data)
	actualHash := hex.EncodeToString(h[:])
	if actualHash != expectedHash {
		return &HashMismatchError{Expected: expectedHash, Actual: actualHash}
	}

	// Hash matches — upload to S3. Use If-None-Match to prevent overwriting
	// an existing blob in case of a TOCTOU race with concurrent imports.
	key := s.key(expectedHash)
	ifNoneMatch := "*"
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		Body:          bytes.NewReader(data),
		ContentLength: new(int64(len(data))),
		IfNoneMatch:   &ifNoneMatch,
	})
	if err != nil {
		// 412 Precondition Failed means the object already exists (If-None-Match: *).
		// Treat as success since content-addressed storage guarantees same hash = same content.
		var respErr interface{ HTTPStatusCode() int }
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 412 {
			return nil
		}
		return fmt.Errorf("put verified: upload: %w", err)
	}

	return nil
}

// Get returns a reader for the blob identified by hash.
// Returns os.ErrNotExist if the blob does not exist.
func (s *S3) Get(ctx context.Context, hash string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    new(s.key(hash)),
	})
	if err != nil {
		if isNoSuchKey(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("get: %w", err)
	}

	return output.Body, nil
}

// Exists reports whether a blob with the given hash exists.
func (s *S3) Exists(ctx context.Context, hash string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    new(s.key(hash)),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("exists: %w", err)
	}

	return true, nil
}

// Delete removes the blob identified by hash.
// If the blob does not exist, Delete is a no-op (S3 does not error on missing keys).
func (s *S3) Delete(ctx context.Context, hash string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    new(s.key(hash)),
	})
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

// isNoSuchKey checks whether the error is an S3 NoSuchKey error.
func isNoSuchKey(err error) bool {
	var nsk *types.NoSuchKey
	return errors.As(err, &nsk)
}

// isNotFound checks whether the error is an S3 NotFound error (from HeadObject).
func isNotFound(err error) bool {
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	// Some S3-compatible services return NoSuchKey for HeadObject too.
	return isNoSuchKey(err)
}
