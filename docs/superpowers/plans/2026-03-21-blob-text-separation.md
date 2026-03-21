# Blob/Text Content Separation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the conflated `content_hash`/`content_length`/`char_count` fields with clean `hash` + `size` fields, and derive text for binary files from chunks instead of a redundant text blob.

**Architecture:** Every file gets one blob (the original content). Text files (markdown) store text as their blob. Binary files (PDF, audio) store the binary as their blob. Extracted/transcribed text lives only in chunks — no separate text blob. `ReadTextFromChunks` concatenates chunk texts for binary file text serving.

**Tech Stack:** Go 1.26, SurrealDB v3, Bubbletea v2

**Spec:** `docs/superpowers/specs/2026-03-21-blob-text-separation-design.md`

---

## File Map

### Models (rename fields)
- `internal/models/file.go` — `File.ContentHash` → `Hash`, `File.ContentLength` → `Size`, drop `CharCount`. Same for `FileInput`, `FileMeta`, `FileEntry`.
- `internal/models/version.go` — `FileVersion.ContentHash` → `Hash`, `VersionInput.ContentHash` → `Hash`
- `internal/models/chunk.go` — `Chunk.DataHash` → `Hash`, `ChunkInput.DataHash` → `Hash`

### DB layer
- `internal/db/schema.go` — Rename fields on `file`, `file_version`, `chunk` tables. Add cleanup REMOVE statements.
- `internal/db/queries_file.go` — Rename `content_hash`/`content_length`/`char_count` → `hash`/`size` in all SQL + params. Remove `fileContentLength()` → `fileSize()`. Remove `UpdateFileTranscript`.
- `internal/db/queries_version.go` — Rename `content_hash` → `hash` in SQL + params.
- `internal/db/queries_chunk.go` — Rename `data_hash` → `hash` in SQL + params.

### Service layer
- `internal/file/service.go` — Remove `storeTranscript()`. Add `ReadTextFromChunks()`. Update `Create()` field names.
- `internal/file/handlers.go` — TranscribeHandler: remove `storeTranscript` call.
- `internal/file/version.go` — Update `maybeCreateVersion` to use `Hash`.

### API / consumers
- `internal/api/assets.go` — `.ContentHash` → `.Hash`, `.ContentLength` → `.Size`
- `internal/api/ls.go` — `.ContentLength` → `.Size`, drop `CharCount`
- `internal/api/types.go` — `AssetMeta.ContentLength` → `Size`, `ContentHash` → `Hash`
- `internal/api/bookmarks.go` — drop `CharCount` from FileEntry construction
- `internal/api/changes.go` — `ContentHash: f.ContentHash` → `.Hash`
- `internal/api/files.go` — `ContentHash: d.ContentHash` → `.Hash`
- `internal/api/export.go` — `f.ContentHash` refs
- `internal/api/export_epub.go` — `f.ContentHash` refs
- `internal/api/import.go` — `ContentHash`, `ContentLength` refs
- `internal/api/bulk.go` — `ContentHash` refs
- `internal/api/versions.go` — `v.ContentHash` → `.Hash`
- `internal/api/openapi.yaml` — Rename fields in all schemas (~8 places)

### Tools (MCP)
- `internal/tools/types.go` — `ContentLength *int` field (MCP tool result meta — keep as-is, separate concept)
- `internal/tools/tool_read_document.go` — `doc.ContentHash` → `.Hash`
- `internal/tools/tool_edit_document.go` — `existing.ContentHash` → `.Hash`
- `internal/tools/tool_edit_document_section.go` — `existing.ContentHash` → `.Hash`
- `internal/tools/tool_get_document_versions.go` — `v.ContentHash` → `.Hash`
- `internal/tools/executor.go` — `checkContentHash` function (keep name, update field access)

### Event system
- `internal/event/bus.go` — `DocumentPayload.ContentHash` → `Hash`
- `internal/event/bus_test.go` — `ContentHash` in test payloads

### File access protocols
- `internal/webdav/fs.go` — `.ContentHash` → `.Hash`, `.ContentLength` → `.Size`
- `internal/webdav/file.go` — `.ContentHash` → `.Hash`
- `internal/nfs/fs.go` — `.ContentLength` → `.Size`
- `internal/sshd/handler.go` — `.ContentLength` → `.Size`

### TUI
- `internal/tui/render.go` — `meta.ContentLength` (MCP tool meta — keep as-is, separate type)

### Pipeline
- `internal/pipeline/pdf.go` — `.ContentLength` → `.Size`
- `internal/pipeline/image.go` — `.ContentLength` → `.Size`
- `internal/file/pdf_handler.go` — `f.ContentHash` → `.Hash`, remove `storeTranscript` call

### Backup
- `internal/backup/backup.go` — `FileInfo.ContentLength` → `Size`, `ContentHash` → `Hash`
- `internal/backup/export.go` — field mappings
- `internal/backup/restore.go` — field mappings

### Tests (all files with changed field refs)
- `internal/db/queries_file_test.go`, `queries_version_test.go`, `queries_chunk_test.go`, `queries_bookmark_test.go`, `queries_vault_test.go`
- `internal/integration/version_test.go`, `mcp_tools_test.go`
- `internal/tools/errors_test.go`
- `internal/event/bus_test.go`
- `internal/tui/app_test.go`

### Note: gorename coverage
`gorename` on model struct fields auto-propagates to most Go consumers. The manual work is SQL strings, JSON tags, and non-model structs (event payload, backup manifest, API types, apiclient types).

### Unchanged
- `models.ContentHash()` function — computes SHA256, not a field rename
- `checkContentHash()` in tools — function name stays, uses renamed field
- `task.ContentHash` — different concept (task text hash), unchanged
- `tools.ToolResultMeta.ContentLength` — MCP display field, separate type

---

## Task 1: Rename File model fields

Use `gorename` for type-safe Go identifier renames. Manual edits for JSON tags.

**Files:**
- Modify: `internal/models/file.go`

- [ ] **Step 1: Remove CharCount from all structs**

Remove the `CharCount` field from `File`, `FileInput`, `FileMeta`, `FileEntry`.

- [ ] **Step 2: Rename ContentHash → Hash using gorename**

First remove old `ContentHash` fields that would conflict, then gorename:
```bash
# File.ContentHash is *string, currently the only ContentHash on File
gorename -from '"github.com/raphi011/know/internal/models".File.ContentHash' -to Hash
gorename -from '"github.com/raphi011/know/internal/models".FileMeta.ContentHash' -to Hash
gorename -from '"github.com/raphi011/know/internal/models".FileEntry.ContentHash' -to Hash  # if exists
```
Note: `FileInput.ContentHash` may need manual rename if gorename fails (check struct).

- [ ] **Step 3: Rename ContentLength → Size using gorename**

```bash
gorename -from '"github.com/raphi011/know/internal/models".File.ContentLength' -to Size
gorename -from '"github.com/raphi011/know/internal/models".FileInput.ContentLength' -to Size
gorename -from '"github.com/raphi011/know/internal/models".FileMeta.ContentLength' -to Size
gorename -from '"github.com/raphi011/know/internal/models".FileEntry.ContentLength' -to Size
```

- [ ] **Step 4: Fix JSON struct tags**

gorename doesn't touch JSON tags. Update manually:
- `json:"content_hash,omitempty"` → `json:"hash,omitempty"`
- `json:"content_length"` → `json:"size"`
- `json:"content_length,omitempty"` → `json:"size,omitempty"`
- Remove any `json:"char_count"` tags

- [ ] **Step 5: Build to find remaining compile errors**

```bash
just build 2>&1 | head -40
```

Fix any pointer-type accesses gorename missed (`.ContentHash` on `*File`, etc.).

- [ ] **Step 6: Commit**

```bash
git add internal/models/file.go
git commit -m "refactor: rename file content fields to hash/size"
```

---

## Task 2: Rename version and chunk model fields

**Files:**
- Modify: `internal/models/version.go`
- Modify: `internal/models/chunk.go`

- [ ] **Step 1: Rename FileVersion.ContentHash → Hash**

```bash
gorename -from '"github.com/raphi011/know/internal/models".FileVersion.ContentHash' -to Hash
```
Also rename `VersionInput.ContentHash` → `Hash`. Fix JSON tag `json:"content_hash"` → `json:"hash"`.

- [ ] **Step 2: Rename Chunk.DataHash → Hash**

```bash
gorename -from '"github.com/raphi011/know/internal/models".Chunk.DataHash' -to Hash
gorename -from '"github.com/raphi011/know/internal/models".ChunkInput.DataHash' -to Hash
```
Fix JSON tags `json:"data_hash,omitempty"` → `json:"hash,omitempty"`.

- [ ] **Step 3: Update IsMultimodal() method**

In `chunk.go`, `IsMultimodal()` references `c.DataHash` → update to `c.Hash`.

- [ ] **Step 4: Build and fix remaining errors**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 5: Commit**

```bash
git add internal/models/version.go internal/models/chunk.go
git commit -m "refactor: rename version content_hash and chunk data_hash to hash"
```

---

## Task 3: Update DB schema

**Files:**
- Modify: `internal/db/schema.go`

- [ ] **Step 1: Update file table fields**

Replace:
```sql
DEFINE FIELD IF NOT EXISTS content_length ON file TYPE int DEFAULT 0;
DEFINE FIELD IF NOT EXISTS char_count     ON file TYPE int DEFAULT 0;
...
DEFINE FIELD IF NOT EXISTS content_hash   ON file TYPE option<string>;
```
With:
```sql
DEFINE FIELD IF NOT EXISTS hash ON file TYPE option<string>;
DEFINE FIELD IF NOT EXISTS size ON file TYPE int DEFAULT 0;
```

- [ ] **Step 2: Update file_version table**

Replace `content_hash` field with `hash option<string>`.

- [ ] **Step 3: Update chunk table**

Replace `data_hash` field with `hash option<string>`.

- [ ] **Step 4: Add cleanup statements**

In the CLEANUP section at the bottom. Note: `REMOVE FIELD size` runs AFTER `DEFINE FIELD size` earlier in the schema — since SurrealDB processes schema DDL sequentially, the DEFINE creates the new field, and the REMOVE only targets old leftovers. However, since `size` is the new field name, we should NOT remove it. Remove only the old names:

```sql
REMOVE FIELD IF EXISTS content_hash ON file;
REMOVE FIELD IF EXISTS content_length ON file;
REMOVE FIELD IF EXISTS char_count ON file;
REMOVE FIELD IF EXISTS content_hash ON file_version;
REMOVE FIELD IF EXISTS data_hash ON chunk;
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go
git commit -m "refactor: update schema for hash/size field rename"
```

---

## Task 4: Update DB queries — file

**Files:**
- Modify: `internal/db/queries_file.go`

- [ ] **Step 1: Rename fileContentLength → fileSize**

Rename the helper function and update its internals (`input.ContentLength` → `input.Size`).

- [ ] **Step 2: Update all SQL SET clauses**

In every CREATE/UPDATE query:
- `content_hash = $content_hash` → `hash = $hash`
- `content_length = $content_length` → `size = $size`
- Remove `char_count = $char_count` lines
- Update parameter maps: `"content_hash"` → `"hash"`, `"content_length"` → `"size"`, etc.

- [ ] **Step 3: Update SELECT column lists**

In `ListFileMetas` and `GetFileMeta*` queries, explicit column lists reference `content_length`, `content_hash`, `char_count` — update to `hash`, `size`.

- [ ] **Step 4: Remove UpdateFileTranscript method**

Delete the `UpdateFileTranscript` function entirely.

- [ ] **Step 5: Update inline SQL for folder creation**

The `ensureFolderExists` and `folderRow` functions have inline SQL with `content_length: 0` — update to `size: 0`. Remove any `char_count` references.

- [ ] **Step 6: Build and fix**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries_file.go
git commit -m "refactor: update file queries for hash/size fields"
```

---

## Task 5: Update DB queries — version and chunk

**Files:**
- Modify: `internal/db/queries_version.go`
- Modify: `internal/db/queries_chunk.go`

- [ ] **Step 1: Update version queries**

All `content_hash` references → `hash` in SQL strings and parameter maps.

- [ ] **Step 2: Update chunk queries**

All `data_hash` references → `hash` in SQL strings and parameter maps.

- [ ] **Step 3: Build and fix**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries_version.go internal/db/queries_chunk.go
git commit -m "refactor: update version and chunk queries for hash field"
```

---

## Task 6: Update file service — remove storeTranscript, add ReadTextFromChunks

**Files:**
- Modify: `internal/file/service.go`
- Modify: `internal/file/handlers.go`
- Modify: `internal/file/version.go` (if exists)

- [ ] **Step 1: Remove storeTranscript method**

Delete the `storeTranscript` function and `import "unicode/utf8"` if no longer needed.

- [ ] **Step 2: Update Create() path**

Rename local variables: `contentHash` → `hash`, `contentLength` → `size`. Remove `charCount`. Update `FileInput` field names in the dbInput construction.

- [ ] **Step 3: Add ReadTextFromChunks**

```go
// ReadTextFromChunks concatenates chunk texts for a file, ordered by position.
// Used to serve text content for binary files that have been processed by the pipeline.
// Returns empty string if no chunks exist.
func (s *Service) ReadTextFromChunks(ctx context.Context, fileID string) (string, error) {
	chunks, err := s.db.GetChunksByFile(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("read text from chunks: %w", err)
	}
	if len(chunks) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for i, c := range chunks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(c.Text)
	}
	return sb.String(), nil
}
```

Check if `GetChunksByFile` already exists or if a similar method can be reused.

- [ ] **Step 4: Remove all storeTranscript call sites**

Remove `storeTranscript` calls from:
- `internal/file/handlers.go` — TranscribeHandler
- `internal/file/pdf_handler.go` — PDF text extraction handler
- `internal/file/service.go` — any remaining call sites (search for `storeTranscript`)

The pipeline should only create chunks — no separate text blob storage.

- [ ] **Step 5: Update maybeCreateVersion**

In `version.go` (or wherever versioning logic lives), update to use `file.Hash` instead of `file.ContentHash`.

- [ ] **Step 6: Update all .ContentHash/.ContentLength refs in service.go**

Fix remaining references: `doc.ContentHash` → `doc.Hash`, `doc.ContentLength` → `doc.Size`, `input.ContentLength` → `input.Size`, etc.

- [ ] **Step 7: Build and fix**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 8: Commit**

```bash
git add internal/file/
git commit -m "refactor: remove storeTranscript, add ReadTextFromChunks, update field names"
```

---

## Task 7: Update API layer, tools, and event system

**Files:**
- Modify: `internal/api/assets.go`, `ls.go`, `types.go`, `bookmarks.go`, `changes.go`, `files.go`, `export.go`, `export_epub.go`, `import.go`, `bulk.go`, `versions.go`
- Modify: `internal/api/openapi.yaml`
- Modify: `internal/tools/tool_read_document.go`, `tool_edit_document.go`, `tool_edit_document_section.go`, `tool_get_document_versions.go`, `executor.go`
- Modify: `internal/event/bus.go`

Note: Many `.ContentHash` → `.Hash` and `.ContentLength` → `.Size` references are auto-fixed by gorename (Task 1). This task covers struct literal field names (`ContentHash:` → `Hash:`), non-model structs (event payload, API types), and JSON tags.

- [ ] **Step 1: Update API types**

In `types.go`: `AssetMeta.ContentLength` → `Size` (json: `"size"`), `ContentHash` → `Hash` (json: `"hash"`).

- [ ] **Step 2: Update all API handlers**

Fix struct literal field names and any remaining `.ContentHash`/`.ContentLength` references across all API handler files: `assets.go`, `ls.go`, `bookmarks.go`, `changes.go`, `files.go`, `export.go`, `export_epub.go`, `import.go`, `bulk.go`, `versions.go`. Drop `CharCount` from FileEntry constructions.

- [ ] **Step 3: Update tools package**

Fix `.ContentHash` → `.Hash` in tool_read_document.go, tool_edit_document.go, tool_edit_document_section.go, tool_get_document_versions.go. Keep `checkContentHash` function name (it describes what it does, not a field).

Note: `tools.ToolResultMeta.ContentLength *int` is a separate MCP display type — leave unchanged.

- [ ] **Step 4: Update event bus**

In `event/bus.go`: rename `DocumentPayload.ContentHash` → `Hash`, update JSON tag.

- [ ] **Step 5: Update OpenAPI spec**

Rename `content_length` → `size`, `content_hash` → `hash`, `contentHash` → `hash`, `contentLength` → `size` in all schemas (~8 places). Remove `char_count`.

- [ ] **Step 6: Build**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 7: Commit**

```bash
git add internal/api/ internal/tools/ internal/event/
git commit -m "refactor: update API, tools, and events for hash/size fields"
```

---

## Task 8: Update file access protocols and pipeline

**Files:**
- Modify: `internal/webdav/fs.go`, `internal/webdav/file.go`
- Modify: `internal/nfs/fs.go`
- Modify: `internal/sshd/handler.go`
- Modify: `internal/pipeline/pdf.go`, `internal/pipeline/image.go`

- [ ] **Step 1: Update WebDAV**

`meta.ContentLength` → `meta.Size`, `asset.ContentHash` → `asset.Hash`, `doc.ContentHash` → `doc.Hash`.

- [ ] **Step 2: Update NFS**

`meta.ContentLength` → `meta.Size`.

- [ ] **Step 3: Update SFTP**

`meta.ContentLength` → `meta.Size`.

- [ ] **Step 4: Update pipeline handlers**

`file.ContentLength` → `file.Size` in pdf.go and image.go.

- [ ] **Step 5: Build**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 6: Commit**

```bash
git add internal/webdav/ internal/nfs/ internal/sshd/ internal/pipeline/
git commit -m "refactor: update protocols and pipeline for hash/size fields"
```

---

## Task 9: Update backup

**Files:**
- Modify: `internal/backup/backup.go`, `internal/backup/export.go`, `internal/backup/restore.go`

- [ ] **Step 1: Update backup manifest struct**

In `backup.go`: `ContentHash` → `Hash` (json: `"hash"`), `ContentLength` → `Size` (json: `"size"`). Remove `CharCount` if present.

- [ ] **Step 2: Update export mapping**

In `export.go`: `f.ContentLength` → `f.Size`, `f.ContentHash` → `f.Hash`.

- [ ] **Step 3: Update restore mapping**

In `restore.go`: `fi.ContentHash` → `fi.Hash`, `fi.ContentLength` → `fi.Size`.

- [ ] **Step 4: Build**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 5: Commit**

```bash
git add internal/backup/
git commit -m "refactor: update backup format for hash/size fields"
```

---

## Task 10: Update apiclient and remote

**Files:**
- Modify: `internal/apiclient/client.go`
- Modify: `internal/remote/executor.go`

- [ ] **Step 1: Update apiclient**

Any references to `ContentHash` or `ContentLength` in request/response types.

- [ ] **Step 2: Update remote executor**

`ContentLength` references in tool result metadata.

- [ ] **Step 3: Build**

```bash
just build 2>&1 | head -40
```

- [ ] **Step 4: Commit**

```bash
git add internal/apiclient/ internal/remote/
git commit -m "refactor: update client and remote for hash/size fields"
```

---

## Task 11: Fix all tests

**Files:**
- Modify: `internal/db/queries_file_test.go`, `internal/db/queries_version_test.go`, `internal/db/queries_chunk_test.go`
- Modify: `internal/db/queries_bookmark_test.go`
- Modify: Any other test files with compile errors

- [ ] **Step 1: Fix test file compile errors**

Update `ContentLength:` → `Size:`, `ContentHash` → `Hash`, `DataHash` → `Hash`, remove `CharCount` references. Remove `UpdateFileTranscript` test (function no longer exists).

Key test files:
- `internal/db/queries_file_test.go` — `ContentLength`, `ContentHash`
- `internal/db/queries_version_test.go` — `ContentHash`
- `internal/db/queries_chunk_test.go` — `DataHash`
- `internal/db/queries_bookmark_test.go` — `ContentLength`
- `internal/db/queries_vault_test.go` — `ContentHash`
- `internal/tools/errors_test.go` — `checkContentHash` test
- `internal/event/bus_test.go` — `ContentHash` in payloads
- `internal/tui/app_test.go` — `ContentLength` (MCP meta — may be unchanged)
- `internal/integration/version_test.go` — `ContentHash`
- `internal/integration/mcp_tools_test.go` — `ContentHash`

- [ ] **Step 2: Build test binaries**

```bash
go test -buildvcs=false ./... -run=^$ 2>&1 | grep "build failed" | head -20
```

- [ ] **Step 3: Run full test suite**

```bash
just test 2>&1 | grep -E "FAIL|^ok"
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: fix tests for hash/size field rename"
```

---

## Task 12: Integration test for ReadTextFromChunks

**Files:**
- Create or modify: `internal/file/service_test.go` or `internal/integration/`

- [ ] **Step 1: Write integration test**

Test that creating a binary file, then creating chunks for it, then calling `ReadTextFromChunks` returns the concatenated text with `\n\n` separators. Also test the empty case (no chunks → empty string).

- [ ] **Step 2: Run test**

```bash
just test
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "test: add ReadTextFromChunks integration test"
```

---

## Task 13: Update documentation

**Files:**
- Modify: `docs/feature-ingestion.md`, `docs/feature-backup.md`
- Modify: any other docs referencing `content_hash`, `content_length`, `data_hash`

- [ ] **Step 1: Search and update docs**

```bash
grep -r "content_hash\|content_length\|data_hash\|char_count" docs/ --include="*.md"
```

Update references to match new field names.

- [ ] **Step 2: Commit**

```bash
git add docs/
git commit -m "docs: update field references for hash/size rename"
```
