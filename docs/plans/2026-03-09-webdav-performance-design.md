# WebDAV Bulk Upload Performance

## Problem

Copying ~50 markdown files via macOS Finder into the WebDAV endpoint takes ~9 minutes. Debug logging revealed:

- **4,880 requests** for 51 files (95 requests/file)
- **98.9% overhead** — only 53 PUTs carry actual content
- **74% PROPFINDs** (3,630), of which 2,658 are `._*` resource fork polling that always returns 404
- Finder is **sequential** at the kernel level (`webdav_fs` kext) — no server-side optimization can parallelize it
- Average 52ms per request; total server time 254s, but only 5.5s on useful work

### Finder's per-file protocol

1. `PROPFIND file.md` (existence check) → 404
2. `PUT file.md Content-Length=0` (claim file) → 201
3. `LOCK` / `UNLOCK` cycle
4. `PROPFIND ._file.md` polling (resource fork) → 404, repeated every ~3s
5. `DELETE file.md` → 204
6. `PUT file.md` (actual content) → 201
7. More `LOCK` / `UNLOCK` / `PROPFIND` cycles

Each of these goes through: auth DB lookup → vault resolution → role check → filesystem operation.

### Root cause

Finder cannot be made faster — it serializes at the kernel level. The only lever is **minimizing per-request latency** on the server.

## Design

Two-phase approach: (A) reduce WebDAV latency, (B) add bulk REST endpoint for CLI.

### Phase A: Minimize Server-Side Latency

#### A1. Fast-path OS metadata files in handler

Short-circuit `._*` and `.DS_Store` requests **before** auth/vault resolution. These are 54% of all requests and currently traverse the full middleware stack.

```
Before: request → DAV header → vault parse → auth → vault resolve → role check → davFS → isOSMetadataFile → 404
After:  request → DAV header → vault parse → isOSMetadataFile → 404/nopFile
```

For PUT/LOCK/UNLOCK on metadata files: return success immediately (nopFile pattern).
For PROPFIND/GET: return 404 immediately.

Saves ~50ms per metadata request × 2,658 requests = ~133s server time.

#### A2. Folder existence cache

`EnsureFolders()` does a DB roundtrip per PUT. When uploading 51 files to `/`, that's 51 identical calls.

- In-memory `sync.Map` in `db.Client` keyed by `(vaultID, folderPath)`
- 60s TTL per entry
- Skip DB call if recently ensured
- Invalidate on folder delete/rename

#### A3. Defer heavy processing to async worker

**Updated**: Heavy processing (chunks, wiki-links, relations) was moved out of `Create()` into `ProcessDocument`, called by a background `ProcessingWorker`. `Create()` now only stores the document with `processed=false`.

### Phase B: Bulk Upload REST Endpoint

For power-user workflows (importing notes folders, migration from Obsidian).

- `POST /api/documents/bulk` — accepts `multipart/form-data`
- Each part has `X-Document-Path` header and markdown body
- Requires `vault` query param
- Returns JSON array of created document IDs + per-file errors
- Server-side: deduplicate `EnsureFolders`, parallel `UpsertDocument` calls
- Wire `knowhow scrape` CLI to use bulk endpoint for multi-file uploads

Not in scope: Finder integration (Finder can't use custom endpoints).

## Research Notes

### Why Finder is slow (fundamental)

- Finder's `webdav_fs` kernel extension serializes all requests per mount
- I/O buffer capped at 8000 bytes (`WEBDAV_MAX_IO_BUFFER_SIZE`)
- No HTTP pipelining, no concurrent uploads
- LOCK support (`DAV: 1, 2`) is mandatory or mount becomes read-only
- The LOCK/UNLOCK/PROPFIND ceremony per file is unavoidable

### Comparison (1000 small files benchmark)

| Client | Time |
|--------|------|
| Cyberduck | 12s |
| cURL + Nextcloud bulk API | 50s |
| cURL 8-parallel | 2m 20s |
| Windows Explorer | 4m 45s |
| rclone | 7m 41s |
| davfs2 (Linux FUSE) | ~17m |
| macOS Finder | ~17m |

Source: https://gist.github.com/joshtrichards/e245aa5cd402b8c0c485a3945ba3ce77

### What production servers do

- Nextcloud: proprietary bulk upload API (`POST /remote.php/dav/bulk`), multipart/related MIME
- All production servers: same `._*` filtering pattern we use
- No server has solved the Finder serialization problem — they all recommend alternative clients for bulk ops

### Useful client-side tip

Disable `.DS_Store` on network volumes:
```bash
defaults write com.apple.desktopservices DSDontWriteNetworkStores true
```

## Expected Impact

- **Phase A**: ~30-50% wall-clock reduction for Finder uploads (from ~9min to ~5min for 50 files)
- **Phase B**: 10-50x faster for CLI bulk imports (parallel + no WebDAV ceremony)
