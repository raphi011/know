# Document Ingestion Pipeline — Architecture

This document covers the internal architecture and implementation details of the ingestion pipeline. For user-facing documentation (frontmatter, query blocks, configuration, CLI usage), see [feature-ingestion.md](feature-ingestion.md).

## Architecture Overview

Documents enter through the CLI (`know import`) or WebDAV file saves. Both paths converge in the **FileService**, which persists the file and enqueues an async **pipeline job**. A single background worker goroutine claims jobs and dispatches them to type-specific handlers. Handlers may chain — for example, the parse handler enqueues an embed job after chunking.

```
  Entry Points              File Service                   Pipeline Worker
 +--------------+        +------------------+          +--------------------+
 | know import  |------->| POST /import/    |          | Single goroutine   |
 | (CLI)        | 2-phase|   manifest       |          | polls every 5s     |
 +--------------+ proto  |   upload         |          | + event-driven wake|
                         +--------+---------+          +--------+-----------+
 +--------------+                 |                             |
 | WebDAV save  |--FileService-->-+                             |
 +--------------+   .Create()    |                             |
                         +--------v---------+          +--------v-----------+
                         | postUpsert()     |---job--->| ClaimJobs(batch)   |
                         |  ensure folders  |  created |  dispatch by type  |
                         |  upsert file     |  event   |  retry on failure  |
                         |  version snapshot|          +--------------------+
                         |  publish event   |                   |
                         |  enqueue job     |          +--------v-----------+
                         +------------------+          |    Job Handlers    |
                                                       | parse | pdf        |
                                                       | transcribe         |
                                                       | summarize | embed  |
                                                       +--------------------+
                                                                |
                                                       +--------v-----------+
                                                       |  Search Indexes    |
                                                       |  BM25 + HNSW       |
                                                       +--------------------+
```

Key design points:
- **Single worker goroutine** replaces three separate processing threads.
- **Event-driven wake**: the worker subscribes to `job.created` on the event bus for instant dispatch, with polling as fallback.
- **All paths converge** through `postUpsert()`, which handles folder creation, versioning, events, and job routing.

## Two-Phase Import Protocol

The `know import` CLI uses a two-phase sync protocol to efficiently transfer files:

```
  Client (know import)                    Server (/api/import)
  ════════════════════                    ════════════════════

  Compute SHA256 hashes locally
        |
        |── POST /import/manifest ──────>│
        |   {vaultId, files: [           │  Check DB: content_hash
        |     {path, hash}, ...          │  for each file
        |   ]}                           │
        |                                │
        |<── {needed: [path1, path3],  ──│
        |     results: [{path, status}]} │
        |                                │
        |── POST /import/upload ────────>│
        |   multipart/form-data          │
        |   Part "meta": {vaultId,       │
        |     hashes: {path: hash}}      │
        |   Part "path1" (binary) ──────>│── stream to blob store
        |   Part "path3" (text) ────────>│── buffer for parsing
        |                                │── verify hash per part
        |                                │── FileService.Create()
        |<── {results: [{path, status}]}─│
```

**Phase 1 — Manifest**: The client sends `{path, hash}` pairs. The server batch-queries existing files (`GetFileMetaByPaths`) and responds with which files need uploading. Unchanged files (matching hash) are skipped without transferring any data.

**Phase 2 — Upload**: Only needed files are sent as multipart. Binary files (images, audio, PDFs) stream directly to the blob store via `BlobStore.PutVerified()` without buffering the entire file in memory. Text files are buffered for markdown parsing. The server verifies each file's hash during upload — if any hash mismatch is detected, the import aborts immediately.

File discovery on the client:
- Inside a git repo: uses `git ls-files` (respects `.gitignore`)
- Outside git: walks the filesystem, filtering dotfiles (override with `--no-ignore`)
- Hash computation parallelized across `min(NumCPU, file_count)` workers

## File Service Layer

### Create() — Text Files

Entry point for markdown and other text files:

1. Validate input (path, vault)
2. Parse frontmatter → extract title, labels, doc_type, summary, metadata
3. Compute SHA256 content hash
4. Normalize path, derive stem for wiki-link matching
5. Call `postUpsert()`

### CreateBinaryFromHash() — Binary Files

Entry point for binary files where the blob is already in the store (from streaming import):

1. Title derived from filename (no markdown parsing)
2. Uses the pre-computed hash from import
3. Call `postUpsert()`

### postUpsert() — Shared Lifecycle

Both entry points converge here:

1. **EnsureFolders()** — create parent folder records in DB
2. **UpsertFile()** — INSERT or UPDATE the file record; returns whether this was a create or update
3. **maybeCreateVersion()** — snapshot previous content if changed (see [Version History](#version-history))
4. **Publish event** — `file.created` or `file.updated` on the event bus
5. **enqueueJob()** — route to the appropriate pipeline job type

### Job Routing

The job type is determined by file properties:

| File Type | Job Type | Example Extensions |
|-----------|----------|--------------------|
| Audio | `transcribe` | `.mp3`, `.wav`, `.m4a`, `.ogg` |
| PDF | `pdf` | `.pdf` |
| Text | `parse` | `.md`, `.txt` |
| Other binary | _(none)_ | `.png`, `.jpg` — stored but not processed |

If the file's content hash is unchanged, no job is enqueued (content already processed).

## Pipeline Job System

### Worker Loop

A single background goroutine runs the pipeline worker:

- **Polling**: calls `ClaimJobs(batch)` every N seconds to atomically claim pending jobs
- **Event-driven**: subscribes to `job.created` on the event bus for immediate wake-up
- **Batch**: processes up to 10 jobs per tick (configurable)
- **Panic recovery**: restarts with 5s delay after any panic

The worker dispatches each claimed job to its registered handler by job type. Unrecognized job types are completed (skipped) with a warning.

### Retry and Failure

When a handler returns an error:

1. If `attempt < max_attempts`: **retry** after 30s delay
2. If `attempt >= max_attempts`: **fail** permanently with the error message

Failed jobs remain in the `pipeline_job` table for debugging. When a file is re-imported (new content), existing pending jobs for that file are superseded by the fresh job.

## Job Handlers

Each handler processes one file type and may enqueue follow-up jobs:

```
  File Type       Initial Job        Follow-up Jobs
  ─────────       ───────────        ──────────────

  Markdown   ──>  parse ──────────>  embed (terminal)

  PDF        ──>  pdf ────────────>  embed (terminal)

  Audio      ──>  transcribe ─────>  embed (terminal)
                       │
                       └────────────> summarize ────> parse ────> embed
                                     (if LLM +        (rechunk    (terminal)
                                      template)        summary)
```

### Parse Handler

Processes markdown files through the full text pipeline:

1. **ParseMarkdown()** — single-pass goldmark AST walk; extracts frontmatter, sections, tasks, wiki-links, @mentions, inline labels (`#tag`), external links, query blocks
2. **Template check** — files under a vault's template path skip heavy processing (only sync labels)
3. **syncChunks()** — run `ChunkMarkdown()`, delete old chunks, create new ones in DB
4. **processWikiLinks()** — resolve `[[links]]` to target files (see [Wiki-Link Resolution](#wiki-link-resolution))
5. **resolveDanglingForFile()** — find and resolve other files' dangling links that now point to this file
6. **handleStemAmbiguity()** — detect when multiple files share a stem, un-resolve ambiguous links
7. **processRelatesTo()** — create graph edges from frontmatter `relates_to` entries
8. **syncTasks()** — extract `- [ ]` / `- [x]` checkboxes into the task table with due dates, labels, heading context
9. **processExternalLinks()** — store extracted URLs with hostname and link text
10. **SyncFileLabels()** — sync all labels (inline + frontmatter) to the label table
11. **Enqueue `embed` job** (if embedder configured)

### PDF Handler

Processes PDF files by rendering pages and extracting text:

1. **CheckPoppler()** — verify `pdftoppm` and `pdftotext` are installed; if missing, the job stays queued for retry
2. **Resolve blob** — get local file path (FS store) or download to temp file (S3 store)
3. **RenderPages()** — `pdftoppm` renders all pages as PNG at configurable DPI (default 300)
4. **Per page**:
   - Read PNG, compute SHA256, store in blob store
   - Extract text: try LLM TextExtractor (vision model on the PNG) first, fall back to `pdftotext` CLI
5. **Create chunks** — one chunk per page with `mime_type="image/png"`, `DataHash` pointing to the PNG blob, `SourceLoc` = "Page N"
6. **Store transcript** — concatenated extracted text saved on the file record for full-text display
7. **Enqueue `embed` job** (if any embedder configured — text or multimodal)

### Transcribe Handler

Processes audio files via speech-to-text:

1. **Load audio** from blob store
2. **STT transcription** — segments with timestamps via configured provider
3. **Group segments** into time-window chunks (default 60s per chunk)
4. **Store transcript** text on the file record
5. **Enqueue `embed` job** (if embedder configured)
6. **Enqueue `summarize` job** (if LLM configured)

### Summarize Handler

Applies a vault template to an audio transcript using an LLM:

1. **Check vault settings** — look up `transcript_template` path; skip if not configured
2. **Fetch template** document from the vault
3. **Apply template variables** — substitute `{{date}}`, `{{path}}`, `{{vault}}` placeholders
4. **LLM FillTemplate()** — generate structured summary from template + raw transcript
5. **Overwrite transcript** with the LLM-rendered summary
6. **Enqueue `parse` job** — rechunk from the summary content (which then enqueues embed)

### Embed Handler (Terminal)

Generates vector embeddings for all un-embedded chunks of a file:

1. **GetUnembeddedChunks()** — fetch chunks missing embeddings
2. **Partition** into text chunks vs multimodal chunks (those with `DataHash` — PDF pages, images)
3. **Text chunks** → text embedder with contextual prefix (`File: {title}\nSection: {heading}\n\n{text}`)
4. **Multimodal chunks** → multimodal embedder operating on the raw image bytes from blob store
5. **Store embeddings** on chunk records in DB
6. **No further jobs** — this is the terminal step

## Chunking Strategy

The chunker splits markdown content into embedding-sized pieces using semantic boundaries:

```
  Content length < Threshold (6000 chars)?
       │
       ├── yes ─> Content < MaxSize (4000)?
       │              │
       │              ├── yes ─> Single chunk (whole document)
       │              └── no ──> Fall through to chunking below
       │
       └── no ──> Has heading sections?
                      │
                      ├── yes ─> One chunk per section
                      │              │
                      │              ├── Section ≤ MaxSize? ──────> Keep as-is
                      │              ├── Code block ≤ 8000 chars? > Keep atomic
                      │              └── Otherwise ────────────────> Split by paragraphs
                      │                                                   │
                      │                                                   ├── Para ≤ MaxSize? > Accumulate to TargetSize
                      │                                                   └── Para > MaxSize? > Split by sentences
                      │
                      └── no ──> Split by paragraphs (same logic)
```

**Parameters** (all configurable via env vars):

| Parameter | Default | Purpose |
|-----------|---------|---------|
| Threshold | 6000 chars | Skip chunking entirely below this |
| TargetSize | 3000 chars (~750 tokens) | Ideal chunk size |
| MaxSize | 4000 chars (~1000 tokens) | Hard limit per chunk |
| Code block limit | min(8000, MaxSize) | Atomic code block ceiling |

**Key behaviors**:
- **Headings included in content**: the heading line (e.g. `## Setup`) is prepended to the section's content so heading terms appear in FTS-indexed chunks
- **No cross-heading merging**: each heading section becomes its own chunk, preserving semantic boundaries
- **Code blocks are atomic**: code-dominated sections stay as one chunk up to 8000 chars (or MaxSize if embedding model has smaller input)
- **Contextual prefix**: before embedding, each chunk gets `File: {title}\nSection: {heading path}\n\n` prepended for retrieval context
- **EffectiveChunkConfig()**: when `KNOW_EMBED_MAX_INPUT_CHARS` is set, MaxSize and TargetSize are automatically capped to leave room for the contextual prefix (~250 chars overhead)

Chunk metadata stored in DB:

| Field | Description |
|-------|-------------|
| `file` | Parent file reference |
| `text` | Chunk content (for BM25 search) |
| `mime_type` | `text/plain` for markdown, `image/png` for PDF pages |
| `position` | Ordering within the document |
| `source_loc` | Location context (e.g. "Page 5" or "0:00-1:05") |
| `data_hash` | SHA256 of binary data (PDF page PNGs) |
| `labels` | Inherited from parent file |
| `embedding` | Vector embedding (populated by embed handler) |

## Wiki-Link Resolution

Wiki-links use `[[target]]` syntax and are resolved using Foam-style stem matching:

1. **Stem matching** — The target is normalized to a stem (lowercase, `.md` stripped, spaces/underscores replaced with hyphens). For example, `[[Beta Notes]]` matches the file `beta-notes.md` because both normalize to the stem `beta-notes`.

2. **Unique stem** — If exactly one file in the vault has the matching stem, the link resolves regardless of folder location. `[[notes]]` resolves to `/deep/nested/notes.md` if it is the only `notes.md` in the vault.

3. **Ambiguous stems** — When multiple files share a stem (e.g. `/a/notes.md` and `/b/notes.md`), a bare `[[notes]]` link stays dangling. To disambiguate, add path segments: `[[a/notes]]`.

4. **Auto-updates on move** — When a file is moved, incoming wiki-link `raw_target` values are recomputed to the shortest unambiguous form.

5. **Ambiguity lifecycle** — Creating a second file with the same stem automatically un-resolves existing stem-only links (making them dangling). Deleting one of the ambiguous files re-resolves the dangling links to the remaining file.

6. **Dangling backfill** — When a new file is created, the parse handler scans for dangling links that now match and resolves them.

## Search Indexing

Ingestion produces two complementary search indexes on the `chunk` table:

### BM25 Fulltext Index

- Index: `idx_chunk_text_ft` on `chunk.text`
- Analyzer: `know_analyzer` — camel tokenizer (splits camelCase at case boundaries), lowercase filter, ASCII normalization, Snowball English stemmer
- Always available, even without an embedder configured

### HNSW Vector Index

- Index: `idx_chunk_embedding` on `chunk.embedding`
- Distance: cosine similarity
- Params: EFC=150, M=12, F32 precision, HASHED_VECTOR
- Dimension: configurable (default 768, must match embedding model output)

**Important**: SurrealDB's HNSW index cannot index `NONE` values. Chunks without embeddings are created with the `embedding` field omitted entirely (not set to `NONE`), so they participate in BM25 search immediately while awaiting the async embed job.

### Hybrid Search

At query time, BM25 and vector results are combined using Reciprocal Rank Fusion (RRF). See `docs/feature-search.md` for details on the search API and ranking.

## Version History

Every document update may create a version snapshot of the previous content:

- **Coalescing**: snapshots are skipped if the last version is less than N minutes old (default 10), preventing version spam during rapid edits
- **Retention**: maximum N versions per file (default 50); oldest pruned when exceeded
- **Immutability**: versions are read-only; rollback creates a new version of the current content, then overwrites with the old content and re-runs the pipeline
- **Vault overrides**: coalesce interval and retention can be configured per-vault via vault settings
