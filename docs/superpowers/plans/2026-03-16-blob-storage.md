# Blob Storage Abstraction — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move binary file data from SurrealDB inline storage to a content-addressed blob store with filesystem and S3 backends.

**Architecture:** New `internal/blob/` package provides a `Store` interface with FS and S3 implementations. Content is addressed by SHA256 hash with 2-level directory sharding. All consumers that currently read `File.Data` switch to `blobStore.Get(ctx, hash)` streaming. DB schema drops the `data` column.

**Tech Stack:** Go stdlib (os, io, crypto/sha256), AWS SDK v2 for S3, SurrealDB

**Spec:** `docs/superpowers/specs/2026-03-16-blob-storage-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/blob/store.go` | `Store` interface, `LocalPathStore` interface, `ShardedKey` helper |
| `internal/blob/fs.go` | Filesystem backend (atomic put, os.Open get, LocalPath) |
| `internal/blob/fs_test.go` | FS backend unit tests |
| `internal/blob/s3.go` | S3-compatible backend (PutObject, GetObject, HeadObject, DeleteObject) |
| `internal/blob/s3_test.go` | S3 backend unit tests (mock HTTP or in-memory) |
| `internal/models/file.go` | Remove `Data []byte` from `File` struct |
| `internal/models/chunk.go` | Remove `Data []byte` from `Chunk` and `ChunkInput` |
| `internal/db/schema.go` | Remove `data` field definitions from file and chunk tables |
| `internal/db/queries_file.go` | Remove `data`/`optionalBytes` from insert/update queries |
| `internal/db/queries_chunk.go` | Remove `data` from chunk creation |
| `internal/db/helpers.go` | Remove `optionalBytes` helper |
| `internal/config/config.go` | Add blob store config fields |
| `internal/file/service.go` | Add blob store field, put blob in Create, update isBinary checks |
| `internal/api/assets.go` | Stream from blob store on download, put blob on upload |
| `internal/api/bulk.go` | Put blob during bulk upload |
| `internal/api/export.go` | Stream blobs into tar during export |
| `internal/webdav/file.go` | Read/write via blob store |
| `internal/nfs/file.go` | Read via blob store for binary files |
| `internal/stt/ffmpeg.go` | Use LocalPath when available |
| `internal/server/bootstrap.go` | Create blob store from config, pass to services |
| `helm/know/templates/pvc.yaml` | New PVC template |
| `helm/know/templates/deployment.yaml` | Add volume mount + blob env vars |
| `helm/know/values.yaml` | Add persistence + blob config sections |

---

### Task 1: Blob Store Interface + FS Implementation

**Files:**
- Create: `internal/blob/store.go`
- Create: `internal/blob/fs.go`
- Create: `internal/blob/fs_test.go`

- [ ] **Step 1: Create store.go with interfaces**

```go
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
```

- [ ] **Step 2: Write failing tests for FS backend**

Write `internal/blob/fs_test.go` with tests:
- `TestFS_PutGet` — put bytes, get back, compare
- `TestFS_PutIdempotent` — put same hash twice, no error
- `TestFS_GetNotFound` — get nonexistent returns `os.ErrNotExist`
- `TestFS_Exists` — true after put, false before
- `TestFS_Delete` — delete removes blob, subsequent get returns not found
- `TestFS_LocalPath` — returns correct sharded path
- `TestShardedKey` — verify sharding format

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/raphaelgruber/Git/know/audio-stt && go test ./internal/blob/...`

- [ ] **Step 4: Implement FS backend**

Create `internal/blob/fs.go`:
- `FS` struct with `dir string` field
- `NewFS(dir string) *FS`
- `Put`: create shard dirs with `os.MkdirAll`, write to temp file in same dir, `os.Rename` (atomic). If target already exists (`Exists`), skip (idempotent).
- `Get`: `os.Open(path)` — wrap `os.ErrNotExist` errors
- `Exists`: `os.Stat`, return false for `os.ErrNotExist`
- `Delete`: `os.Remove`, ignore `os.ErrNotExist`
- `LocalPath`: return `filepath.Join(dir, ShardedKey(hash))`
- Ensure `FS` implements both `Store` and `LocalPathStore` (compile-time check)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/blob/...`

- [ ] **Step 6: Commit**

```
feat: add blob store interface and filesystem backend
```

---

### Task 2: S3 Backend

**Files:**
- Create: `internal/blob/s3.go`
- Create: `internal/blob/s3_test.go`

- [ ] **Step 1: Write S3 backend**

Create `internal/blob/s3.go`:
- `S3` struct with `client *s3.Client`, `bucket string`, `prefix string`
- `NewS3(client *s3.Client, bucket, prefix string) *S3`
- `key(hash)` helper: `filepath.Join(prefix, ShardedKey(hash))`
- `Put`: `s3.PutObject` with `Body: r`, `ContentLength: size`. Check if exists first (idempotent).
- `Get`: `s3.GetObject`, return `output.Body`. Convert `NoSuchKey` error to `os.ErrNotExist`.
- `Exists`: `s3.HeadObject`, return false for `NoSuchKey`.
- `Delete`: `s3.DeleteObject` (S3 doesn't error on missing keys).
- Import: `github.com/aws/aws-sdk-go-v2/service/s3` (already in go.mod)
- Compile-time check: `var _ Store = (*S3)(nil)`

- [ ] **Step 2: Write tests**

Create `internal/blob/s3_test.go`:
- Use `httptest.NewServer` to mock S3 API, OR
- Skip tests with `t.Skip("S3 tests require endpoint")` if no test endpoint configured
- Test Put/Get/Exists/Delete same as FS tests but through S3 mock

- [ ] **Step 3: Run tests**

Run: `go test ./internal/blob/...`

- [ ] **Step 4: Commit**

```
feat: add S3-compatible blob store backend
```

---

### Task 3: Config + Bootstrap Wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/server/bootstrap.go`

- [ ] **Step 1: Add config fields**

In `internal/config/config.go`, add to Config struct:

```go
// Blob storage
BlobStore    string // KNOW_BLOB_STORE (default: "fs")
BlobDir      string // KNOW_BLOB_DIR (default: "/data/blobs")
BlobS3Bucket string // KNOW_BLOB_S3_BUCKET
BlobS3Prefix string // KNOW_BLOB_S3_PREFIX (default: "blobs")
BlobS3Endpoint string // KNOW_BLOB_S3_ENDPOINT
BlobS3Region   string // KNOW_BLOB_S3_REGION (default: "us-east-1")
```

Add loading in `Load()`:

```go
BlobStore:      getEnv("KNOW_BLOB_STORE", "fs"),
BlobDir:        getEnv("KNOW_BLOB_DIR", "/data/blobs"),
BlobS3Bucket:   getEnv("KNOW_BLOB_S3_BUCKET", ""),
BlobS3Prefix:   getEnv("KNOW_BLOB_S3_PREFIX", "blobs"),
BlobS3Endpoint: getEnv("KNOW_BLOB_S3_ENDPOINT", ""),
BlobS3Region:   getEnv("KNOW_BLOB_S3_REGION", "us-east-1"),
```

- [ ] **Step 2: Wire blob store in bootstrap.go**

In `New()`, after DB init, before creating file service:

```go
blobStore, err := createBlobStore(cfg)
if err != nil {
    return nil, fmt.Errorf("create blob store: %w", err)
}
```

Add helper:
```go
func createBlobStore(cfg config.Config) (blob.Store, error) {
    switch cfg.BlobStore {
    case "fs":
        return blob.NewFS(cfg.BlobDir), nil
    case "s3":
        // Create S3 client with optional custom endpoint
        // Use aws-sdk-go-v2 config loading
        return blob.NewS3(client, cfg.BlobS3Bucket, cfg.BlobS3Prefix), nil
    default:
        return nil, fmt.Errorf("unknown blob store: %q", cfg.BlobStore)
    }
}
```

Add `blobStore blob.Store` field to `App` struct. Pass to file service.

Log on startup: `slog.Info("blob store configured", "type", cfg.BlobStore)`

- [ ] **Step 3: Build to verify**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 4: Commit**

```
feat: add blob store config and bootstrap wiring
```

---

### Task 4: Remove Data from DB Schema + Models

**Files:**
- Modify: `internal/models/file.go`
- Modify: `internal/models/chunk.go`
- Modify: `internal/db/schema.go`
- Modify: `internal/db/queries_file.go`
- Modify: `internal/db/queries_chunk.go`
- Modify: `internal/db/helpers.go`

- [ ] **Step 1: Remove Data from File model**

In `internal/models/file.go`:
- Remove `Data []byte` field from `File` struct
- Keep `Data []byte` on `FileInput` (transient during upload)

- [ ] **Step 2: Remove Data from Chunk model**

In `internal/models/chunk.go`:
- Remove `Data []byte` from `Chunk` struct
- Remove `Data []byte` from `ChunkInput` struct

- [ ] **Step 3: Remove data from DB schema**

In `internal/db/schema.go`:
- Remove the `DEFINE FIELD IF NOT EXISTS data` line from file table
- Remove the `DEFINE FIELD IF NOT EXISTS data` line from chunk table

- [ ] **Step 4: Remove data from DB queries**

In `internal/db/queries_file.go`:
- Remove `data = $data` / `optionalBytes(input.Data)` from `CreateFile` and `UpdateFile` SQL
- Remove `data` from the vars maps
- Update `fileSize()` helper — size is now always `len(input.Data)` if `Data` is set, else `len(input.Content)` (no change needed, it still works from `FileInput.Data`)

In `internal/db/queries_chunk.go`:
- Remove `data` / `optionalBytes` from chunk creation queries

In `internal/db/helpers.go`:
- Remove `optionalBytes` function (no longer used)

- [ ] **Step 5: Fix all compilation errors**

Run: `go build -buildvcs=false ./...`

Fix any remaining references to `File.Data` or `Chunk.Data` — these are the consumers that need updating in subsequent tasks.

- [ ] **Step 6: Commit**

```
refactor: remove binary data from DB schema and models
```

---

### Task 5: Update File Service — Blob Store Integration

**Files:**
- Modify: `internal/file/service.go`

- [ ] **Step 1: Add blob store to Service**

Add `blobStore blob.Store` field to Service struct. Update `NewService` to accept it. Add `BlobStore() blob.Store` accessor for consumers (API handlers, WebDAV, NFS).

- [ ] **Step 2: Update Create() — put blob before DB write**

In `Create()`, after computing content hash for binary files:
```go
if len(input.Data) > 0 {
    hash := contentHash(input.Data) // already computed
    if err := s.blobStore.Put(ctx, hash, bytes.NewReader(input.Data), int64(len(input.Data))); err != nil {
        return nil, fmt.Errorf("store blob: %w", err)
    }
}
```

- [ ] **Step 3: Update isBinary detection**

Replace `isBinary := len(doc.Data) > 0` with checking MimeType or ContentHash:
```go
isBinary := doc.ContentHash != nil && doc.Size > 0
```

Or check MimeType against known binary types. Choose the approach that's most reliable given the existing code.

- [ ] **Step 4: Build and test**

Run: `go build -buildvcs=false ./...` and `go test ./internal/file/...`

- [ ] **Step 5: Commit**

```
feat: integrate blob store into file service create flow
```

---

### Task 6: Update API Handlers — Assets, Bulk, Export

**Files:**
- Modify: `internal/api/assets.go`
- Modify: `internal/api/bulk.go`
- Modify: `internal/api/export.go`

- [ ] **Step 1: Update asset download**

In `getAsset` handler: replace `bytes.NewReader(asset.Data)` with:
```go
reader, err := blobStore.Get(ctx, *asset.ContentHash)
if err != nil { ... }
defer reader.Close()
// Stream to response
```

The handler needs access to the blob store — add it to the API server/handler struct, passed from bootstrap.

- [ ] **Step 2: Update asset upload**

In `uploadAsset`: put blob before creating file record.

- [ ] **Step 3: Update bulk upload**

In `processBulkAsset`: put blob, then create file without Data in DB.

- [ ] **Step 4: Update export**

In export handler: for binary files, stream from blob store into tar instead of reading `f.Data`.

- [ ] **Step 5: Build and test**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 6: Commit**

```
feat: stream binary files from blob store in API handlers
```

---

### Task 7: Update WebDAV + NFS

**Files:**
- Modify: `internal/webdav/file.go`
- Modify: `internal/nfs/file.go`

- [ ] **Step 1: Update WebDAV read**

Replace `bytes.Reader(f.Data)` with `blobStore.Get(ctx, *f.ContentHash)`. The WebDAV handler needs the blob store — pass it through the WebDAV filesystem constructor.

- [ ] **Step 2: Update WebDAV write**

On close/flush: compute hash, put blob, then create file record.

- [ ] **Step 3: Update NFS read**

For binary files: stream from blob store. NFS handler needs blob store reference.

- [ ] **Step 4: Build and test**

Run: `go build -buildvcs=false ./...`

- [ ] **Step 5: Commit**

```
feat: serve binary files from blob store via WebDAV and NFS
```

---

### Task 8: Update STT ffmpeg Integration

**Files:**
- Modify: `internal/stt/ffmpeg.go`
- Modify: `internal/file/service.go` (transcribeFile method)

- [ ] **Step 1: Update ffmpeg to use LocalPath**

In `SplitForTranscription`, accept a file path instead of `[]byte data`:
```go
func SplitForTranscription(ctx context.Context, inputPath string, maxBytes int) ([]SplitPart, error)
```

Or add a variant that accepts a path. The caller checks for `LocalPathStore` and passes the path directly.

- [ ] **Step 2: Update transcribeFile in service.go**

```go
func (s *Service) transcribeFile(ctx context.Context, transcriber stt.Transcriber, f *models.File, fileID string) error {
    var audioData []byte
    if local, ok := s.blobStore.(blob.LocalPathStore); ok {
        path := local.LocalPath(*f.ContentHash)
        // For small files: read and pass to transcriber
        // For large files: pass path to ffmpeg directly
    } else {
        // S3: stream to temp file
        rc, err := s.blobStore.Get(ctx, *f.ContentHash)
        // ...
    }
}
```

- [ ] **Step 3: Build and test**

Run: `go build -buildvcs=false ./...` and `go test ./internal/stt/...`

- [ ] **Step 4: Commit**

```
feat: use blob store LocalPath for ffmpeg direct file access
```

---

### Task 9: Helm Chart — PVC + Volume Mount

**Files:**
- Create: `helm/know/templates/pvc.yaml`
- Modify: `helm/know/templates/deployment.yaml`
- Modify: `helm/know/values.yaml`

- [ ] **Step 1: Create PVC template**

Create `helm/know/templates/pvc.yaml`:
```yaml
{{- if .Values.persistence.enabled }}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "know.fullname" . }}-data
  labels:
    {{- include "know.labels" . | nindent 4 }}
spec:
  accessModes:
    - {{ .Values.persistence.accessMode | default "ReadWriteOnce" }}
  resources:
    requests:
      storage: {{ .Values.persistence.size | default "10Gi" }}
  {{- if .Values.persistence.storageClass }}
  storageClassName: {{ .Values.persistence.storageClass }}
  {{- end }}
{{- end }}
```

- [ ] **Step 2: Add volume mount to deployment**

In `deployment.yaml`, add to container spec:
```yaml
volumeMounts:
  - name: data
    mountPath: /data
```

Add to pod spec:
```yaml
volumes:
  {{- if .Values.persistence.enabled }}
  - name: data
    persistentVolumeClaim:
      claimName: {{ include "know.fullname" . }}-data
  {{- else }}
  - name: data
    emptyDir: {}
  {{- end }}
```

Add blob env vars:
```yaml
- name: KNOW_BLOB_STORE
  value: {{ .Values.blob.store | default "fs" | quote }}
- name: KNOW_BLOB_DIR
  value: {{ .Values.blob.dir | default "/data/blobs" | quote }}
```

Add S3 env vars conditionally (when `blob.store == "s3"`).

- [ ] **Step 3: Add values**

In `values.yaml`, add:
```yaml
persistence:
  enabled: true
  size: 10Gi
  accessMode: ReadWriteOnce
  storageClass: ""

blob:
  store: fs
  dir: /data/blobs
  s3:
    bucket: ""
    prefix: blobs
    endpoint: ""
    region: us-east-1
```

- [ ] **Step 4: Validate Helm template**

Run: `helm template test helm/know/` — verify rendered YAML is correct.

- [ ] **Step 5: Commit**

```
feat: add persistent volume and blob store config to Helm chart
```

---

### Task 10: Final Verification

- [ ] **Step 1: Full build**

Run: `just build`

- [ ] **Step 2: Full tests**

Run: `just test`

- [ ] **Step 3: Manual end-to-end**

```bash
just bootstrap && just dev
just run cp ./docs / --vault default
# Verify blobs on disk: ls /data/blobs/ (or configured dir)
# Verify REST: curl asset download streams correctly
# Verify no data column in DB: SELECT data FROM file (should error or return NONE)
```
