package blob

import (
	"context"
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
