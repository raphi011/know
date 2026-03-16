package blob_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/raphi011/know/internal/blob"
)

func newS3TestStore(t *testing.T) *blob.S3 {
	t.Helper()

	endpoint := os.Getenv("KNOW_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3 tests require KNOW_TEST_S3_ENDPOINT")
	}

	bucket := os.Getenv("KNOW_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "test-blobs"
	}

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// Ensure the test bucket exists.
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		// Ignore BucketAlreadyOwnedByYou / BucketAlreadyExists errors.
		t.Logf("create bucket (may already exist): %v", err)
	}

	prefix := "test-" + t.Name()
	return blob.NewS3(client, bucket, prefix)
}

func TestS3_PutGet(t *testing.T) {
	store := newS3TestStore(t)
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

func TestS3_PutIdempotent(t *testing.T) {
	store := newS3TestStore(t)
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

func TestS3_GetNotFound(t *testing.T) {
	store := newS3TestStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent000000")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestS3_Exists(t *testing.T) {
	store := newS3TestStore(t)
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

func TestS3_Delete(t *testing.T) {
	store := newS3TestStore(t)
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

	// Delete nonexistent should not error.
	err = store.Delete(ctx, "nonexistent000000")
	if err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}
