# Blob/Text Content Separation

## Problem

The file table uses a single `content_hash` field for both binary data (PDFs, audio, images) and text content (markdown). When the pipeline extracts text from a binary file, `storeTranscript` overwrites the binary hash with the text hash — losing the reference to the original file. The extracted text is also redundantly stored as both a separate blob and as chunks.

## Design

### Principle

Every file has one blob in the blob store — the original uploaded content. Text content is either the blob itself (for text files like markdown) or derived from chunks (for binary files after pipeline processing). There is no separate "text blob" for extracted/transcribed content.

### File table fields

Replace `content_hash option<string>`, `content_length int`, `char_count int` with:

| Field | Type | Description |
|---|---|---|
| `hash` | `option<string>` | SHA256 of original file content in blob store |
| `size` | `int` | Byte size of original file content |

The existing `mime_type` field determines whether the blob is text or binary.

### Content retrieval

| File type | How to get text | How to get binary |
|---|---|---|
| Text file (`IsTextFile`) | Read blob by `hash` as string | n/a |
| Binary (processed) | Concatenate chunk texts | Read blob by `hash` |
| Binary (unprocessed) | No text available | Read blob by `hash` |
| Folder | n/a | n/a |

### file_version table

Replace `content_hash string` with `hash option<string>`. Versions snapshot the blob hash. Version creation triggers when `hash` changes.

### chunk table

Rename `data_hash` to `hash`. This references binary data in the blob store for multimodal chunks (e.g. images extracted from PDFs). The inline `text` field is unchanged.

### Removed: storeTranscript

The `storeTranscript()` method and `UpdateFileTranscript()` DB method are removed. The pipeline creates chunks directly without storing a separate text blob. The file's `hash` always points to the original binary.

### New: ReadTextFromChunks

```go
func (s *Service) ReadTextFromChunks(ctx context.Context, fileID string) (string, error)
```

Concatenates chunk texts ordered by position, separated by `\n\n`. Used by API/WebDAV/NFS/SFTP when serving text for binary files.

If chunks haven't been created yet (pipeline hasn't run), returns empty string. This matches current behavior — binary files have no text until pipeline processes them.

### Version rollback

Rollback only applies to text files (markdown). For binary files, the blob doesn't change — only chunks change during re-extraction, and chunk-level versioning is out of scope. Re-extraction with a better model (same binary, different chunks) does not create a version.

### WebDAV/NFS serving binary-file text

When serving text for a processed binary file (via ReadTextFromChunks), the text must be assembled before serving. WebDAV `Stat` returns `blob_size` for binary files — the actual served content length may differ when text is requested. This is acceptable since binary-file text serving is a secondary use case.

### Backup format

Manifest uses `hash` + `size` per file. Both the blob and chunk data can be reconstructed: blob from the backup archive, chunks by re-running the pipeline after restore. Old backup archives are incompatible (no production users).

### Unchanged: task content_hash

The `task` table has a `content_hash` field for stable task identity (hash of task text). This is semantically different from file content hashing and is unchanged by this refactor.

## Population examples

| Scenario | `hash` | `size` |
|---|---|---|
| Upload `notes.md` (1200 bytes) | `sha256("# Notes\n...")` | 1200 |
| Upload `report.pdf` (2.4MB) | `sha256(<pdf bytes>)` | 2516582 |
| Upload `meeting.mp3` (8MB) | `sha256(<mp3 bytes>)` | 8388608 |
| Create folder `/docs/` | null | 0 |
| Re-upload updated PDF | new `sha256(<new pdf>)` | new size |

After pipeline processes `report.pdf`: `hash` and `size` unchanged. Text lives in chunks.
