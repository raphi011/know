# Blob Storage Abstraction

## Context

Binary file data (images, audio, PDFs) is currently stored inline in SurrealDB as `option<bytes>` on the `file` table. This causes:
- Every `SELECT` on the file table loads full binary data into memory, even when only metadata is needed
- No streaming — entire blobs buffered in Go before serving
- STT/ffmpeg requires writing DB bytes to temp files, then reading them back
- SurrealDB is not optimized for large blob storage

This spec introduces a blob storage abstraction with local filesystem and S3-compatible backends. Binary data moves out of SurrealDB into a dedicated store, addressed by content hash (SHA256).

## Design Decisions

- **Content-addressed** — blobs keyed by SHA256 hash (already computed as `content_hash`). Natural dedup, immutable.
- **Streaming API** — `io.Reader`/`io.ReadCloser` to avoid loading entire blobs into memory.
- **Two backends** — local filesystem (dev/single-node) and S3-compatible (cloud/production).
- **No migration** — DB will be wiped and data reimported. Clean cut.
- **No GC** — orphaned blobs tolerated for now. Add CLI command later.
- **FS direct path** — local filesystem backend exposes file paths so ffmpeg can read directly.

## Interface

```go
// internal/blob/store.go
package blob

type Store interface {
    // Put stores a blob. If a blob with this hash already exists, it's a no-op.
    Put(ctx context.Context, hash string, r io.Reader, size int64) error

    // Get returns a reader for the blob. Returns os.ErrNotExist if not found.
    Get(ctx context.Context, hash string) (io.ReadCloser, error)

    // Exists checks whether a blob exists.
    Exists(ctx context.Context, hash string) (bool, error)

    // Delete removes a blob. No error if it doesn't exist.
    Delete(ctx context.Context, hash string) error
}

// LocalPathStore is optionally implemented by backends that store blobs
// on the local filesystem. Allows direct file access (e.g., for ffmpeg).
type LocalPathStore interface {
    Store
    LocalPath(hash string) string
}
```

## Blob Path Layout

Both backends use 2-level hash sharding: `{hash[0:2]}/{hash[2:4]}/{hash}`

- FS: `/data/blobs/ab/cd/abcdef1234...`
- S3: `s3://{bucket}/{prefix}/ab/cd/abcdef1234...`

## Implementations

### `blob.NewFS(dir string) *FS`

File: `internal/blob/fs.go`

- `Put`: write to `{dir}/{shard}/{hash}` via temp file + rename (atomic)
- `Get`: `os.Open` the path, return as `io.ReadCloser`
- `Exists`: `os.Stat`
- `Delete`: `os.Remove`
- `LocalPath`: returns the full filesystem path
- Implements both `Store` and `LocalPathStore`

### `blob.NewS3(client *s3.Client, bucket, prefix string) *S3`

File: `internal/blob/s3.go`

- `Put`: `s3.PutObject` with body from `io.Reader`
- `Get`: `s3.GetObject`, return `Body` as `io.ReadCloser`
- `Exists`: `s3.HeadObject`
- `Delete`: `s3.DeleteObject`
- Uses AWS SDK v2 (already in go.mod for Bedrock)
- S3-compatible services (MinIO, R2) via custom endpoint URL

## Configuration

```
KNOW_BLOB_STORE=fs|s3              (default: "fs")
KNOW_BLOB_DIR=/data/blobs          (for fs, default: "/data/blobs")
KNOW_BLOB_S3_BUCKET=               (for s3, required)
KNOW_BLOB_S3_PREFIX=blobs          (for s3, default: "blobs")
KNOW_BLOB_S3_ENDPOINT=             (for s3-compatible, e.g., http://minio:9000)
KNOW_BLOB_S3_REGION=us-east-1      (for s3, default: "us-east-1")
```

## DB Schema Changes

Remove from `file` table:
```sql
-- REMOVE: DEFINE FIELD IF NOT EXISTS data ON file TYPE option<bytes>;
```

Remove from `chunk` table:
```sql
-- REMOVE: DEFINE FIELD IF NOT EXISTS data ON chunk TYPE option<bytes>;
```

Keep `content_hash` and `size` on `file` — these are the blob reference and metadata.

## Go Model Changes

### `models.File`

- Remove `Data []byte` field from `File` struct
- Keep `ContentHash *string` and `Size int`

### `models.FileInput`

- Keep `Data []byte` field — used transiently during upload (compute hash, put blob)
- `Data` is never persisted to DB; only `ContentHash` and `Size` are stored

### `models.Chunk`

- Remove `Data []byte` field (was for multimodal embedding, unused)

## Consumer Migration

| Consumer | File | Current | New |
|----------|------|---------|-----|
| WebDAV read | `internal/webdav/file.go` | `bytes.Reader(f.Data)` | `blobStore.Get(ctx, *f.ContentHash)` |
| WebDAV write | `internal/webdav/file.go` | `Create(Data: buf)` | `blobStore.Put(hash, buf)` then `Create()` |
| NFS read | `internal/nfs/file.go` | `bytes.NewReader(data)` | `blobStore.Get(ctx, hash)` — only for binary |
| Asset download | `internal/api/assets.go` | `http.ServeContent(f.Data)` | Stream from `blobStore.Get` |
| Asset upload | `internal/api/assets.go` | `Create(Data: data)` | `blobStore.Put` then `Create()` |
| Bulk upload | `internal/api/bulk.go` | reads bytes, sets `Data` | `blobStore.Put` then create |
| Export | `internal/api/export.go` | reads `f.Data` | `blobStore.Get` per file, stream to tar |
| STT transcribe | `internal/file/service.go` | reads `f.Data` → temp file | `blobStore.LocalPath(hash)` or stream |
| File service | `internal/file/service.go` | `isBinary = len(doc.Data) > 0` | `isBinary = file.ContentHash != nil && file.Size > 0` (or check MimeType) |

## Create Flow (Binary File)

1. Caller provides `FileInput` with `Data []byte`
2. `Service.Create()` computes `content_hash = SHA256(data)`, `size = len(data)`
3. `blobStore.Put(ctx, hash, bytes.NewReader(data), size)` — idempotent (dedup)
4. `db.UpsertFile()` stores metadata only (path, hash, size, mime_type) — no `data` column
5. `FileInput.Data` is transient, never hits DB

## Read Flow (Binary File)

1. `db.GetFileByPath()` returns `File` with `ContentHash` and `Size`
2. Consumer calls `blobStore.Get(ctx, *file.ContentHash)` for an `io.ReadCloser`
3. Streams to client

## STT Integration

For local filesystem backend:
```go
if local, ok := blobStore.(blob.LocalPathStore); ok {
    path := local.LocalPath(*file.ContentHash)
    // ffmpeg reads directly — no copy needed
} else {
    // S3: stream to temp file, then ffmpeg
}
```

## Helm Chart Changes

### New: `templates/pvc.yaml`

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "know.fullname" . }}-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: {{ .Values.persistence.size | default "10Gi" }}
  {{- if .Values.persistence.storageClass }}
  storageClassName: {{ .Values.persistence.storageClass }}
  {{- end }}
```

### Modify: `templates/deployment.yaml`

- Add volume mount at `/data`:
  ```yaml
  volumeMounts:
    - name: data
      mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: {{ include "know.fullname" . }}-data
  ```
- Keep `readOnlyRootFilesystem: true` — `/data` is a separate mount
- Add blob config env vars (`KNOW_BLOB_STORE`, `KNOW_BLOB_DIR`)

### Modify: `values.yaml`

```yaml
persistence:
  enabled: true
  size: 10Gi
  storageClass: ""  # uses default

blob:
  store: fs          # fs or s3
  dir: /data/blobs
  s3:
    bucket: ""
    prefix: blobs
    endpoint: ""
    region: us-east-1
```

## Bootstrap Wiring

In `internal/server/bootstrap.go`:
1. Create blob store from config (FS or S3)
2. Pass to `file.Service` (new constructor parameter or setter)
3. Pass to WebDAV, NFS, and API handlers that serve blobs
4. Log blob store type on startup

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/blob/store.go` | **New** — Store interface, LocalPathStore |
| `internal/blob/fs.go` | **New** — filesystem implementation |
| `internal/blob/fs_test.go` | **New** — tests |
| `internal/blob/s3.go` | **New** — S3 implementation |
| `internal/blob/s3_test.go` | **New** — tests (with mock/localstack) |
| `internal/models/file.go` | **Modify** — remove Data from File, keep on FileInput |
| `internal/models/chunk.go` | **Modify** — remove Data |
| `internal/db/schema.go` | **Modify** — remove data columns |
| `internal/db/queries_file.go` | **Modify** — remove data from queries |
| `internal/db/queries_chunk.go` | **Modify** — remove data from queries |
| `internal/file/service.go` | **Modify** — add blob store, update Create/ProcessFile |
| `internal/api/assets.go` | **Modify** — stream from blob store |
| `internal/api/bulk.go` | **Modify** — put blobs, create without data |
| `internal/api/export.go` | **Modify** — stream blobs into tar |
| `internal/webdav/file.go` | **Modify** — read/write via blob store |
| `internal/nfs/file.go` | **Modify** — read via blob store |
| `internal/stt/ffmpeg.go` | **Modify** — use LocalPath when available |
| `internal/config/config.go` | **Modify** — add blob config fields |
| `internal/server/bootstrap.go` | **Modify** — create blob store, wire into services |
| `helm/know/templates/pvc.yaml` | **New** — persistent volume claim |
| `helm/know/templates/deployment.yaml` | **Modify** — volume mount, blob env vars |
| `helm/know/values.yaml` | **Modify** — persistence + blob config |

## Verification

1. `just build` — compiles
2. `just test` — all tests pass
3. Unit tests for FS and S3 blob stores
4. Manual: `just bootstrap && just dev && just run cp ./docs / --vault default` — files stored on disk, served via REST/WebDAV
5. Verify blobs exist at `/data/blobs/{hash[0:2]}/{hash[2:4]}/{hash}`
6. Verify `SELECT data FROM file` returns NONE for all records
