# Document Ingestion Pipeline

The ingestion pipeline handles importing, parsing, embedding, and indexing documents into know vaults. It processes markdown, PDFs, and audio files through an async job-based pipeline that extracts metadata, resolves wiki-links, generates vector embeddings, and builds search indexes.

## Technical Reference

For architecture details, job handlers, chunking strategy, wiki-link resolution, search indexing, and the two-phase import protocol, see [tech-ingestion.md](tech-ingestion.md).

## Frontmatter

Documents support YAML frontmatter for structured metadata:

```yaml
---
type: document
title: Auth Service
labels: [work, infrastructure]
summary: Handles authentication and tokens
verified: true
---
```

Frontmatter fields:

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Document title (overrides filename) |
| `labels` | string[] | Classification labels |
| `type` | string | Document type (filters in search/API) |

Any other frontmatter keys (e.g. `summary`, `verified`) are stored as generic metadata on the document.

## Query Blocks

Documents can embed live queries using an inline DSL inside `know` code blocks. Queries must start with a format keyword (`LIST`, `TABLE`, or `TASK`):

````markdown
```know
TABLE title, summary AS "Summary"
FROM /projects
WHERE labels CONTAIN "active"
SORT updated_at DESC
LIMIT 10
```
````

See [Query Blocks in feature-render.md](feature-render.md#query-blocks) for full syntax reference.

## Graceful Degradation

All AI components are optional. The system works fully without any AI configured — files are stored, parsed, and searchable via BM25:

| Component | Env Var | When Missing |
|-----------|---------|--------------|
| Text Embedder | `KNOW_EMBED_PROVIDER` | No vector search; BM25 fulltext still works |
| Multimodal Embedder | `KNOW_MULTIMODAL_EMBED_PROVIDER` | PDF/image chunks use text-only embedding of extracted text |
| STT Transcriber | `KNOW_STT_PROVIDER` | Audio files stored but not transcribed |
| LLM Model | `KNOW_LLM_PROVIDER` | No transcript summarization; raw transcripts kept as-is |
| Text Extractor | `KNOW_TEXT_EXTRACTOR_MODEL` | PDF text from `pdftotext` CLI only (no LLM vision OCR) |
| Poppler | system package | PDF jobs stay queued, retry until installed |

## Usage

### Importing Files

```bash
# Import a single file
know import ./speech.mp3 / --vault default

# Import top-level files from a directory (unchanged files automatically skipped)
know import ./docs / --vault default

# Recursive import with labels
know import ./notes /notes --vault default -r --labels "personal"

# Dry run (preview which files would be imported)
know import ./wiki /wiki --vault default --dry-run

# Force overwrite files with different content hash
know import ./docs /docs --vault default --force
```

Writing a file through the WebDAV interface also triggers the full pipeline on save.

## Configuration Reference

### Pipeline Worker

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_PIPELINE_WORKER_INTERVAL` | `5` | Seconds between worker poll ticks |
| `KNOW_PIPELINE_WORKER_BATCH` | `10` | Max jobs claimed per tick |
| `KNOW_INGEST_CONCURRENCY` | `4` | Concurrent file processing during import |

### Chunking

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_CHUNK_THRESHOLD` | `6000` | Only chunk if content exceeds this (chars) |
| `KNOW_CHUNK_TARGET_SIZE` | `3000` | Ideal chunk size (chars, ~750 tokens) |
| `KNOW_CHUNK_MAX_SIZE` | `4000` | Hard limit per chunk (chars, ~1000 tokens) |
| `KNOW_EMBED_MAX_INPUT_CHARS` | `0` | Max chars per embed API call (0 = no limit); auto-caps chunk sizes |

### Embedding

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_EMBED_PROVIDER` | `none` | Text embedding provider (`openai`, `ollama`, `bedrock`, `googleai`) |
| `KNOW_EMBED_MODEL` | _(none)_ | Embedding model name |
| `KNOW_EMBED_DIMENSION` | `768` | Vector dimension (must match model output) |
| `KNOW_MULTIMODAL_EMBED_PROVIDER` | `none` | Multimodal embedding provider (currently `googleai`) |
| `KNOW_MULTIMODAL_EMBED_MODEL` | _(none)_ | Multimodal embedding model name |

### PDF Processing

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_TEXT_EXTRACTOR_MODEL` | `gemini-2.0-flash` | LLM model for PDF page text extraction |
| `KNOW_PDF_RENDER_DPI` | `300` | DPI for PDF page rendering via poppler |

### Speech-to-Text

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_STT_PROVIDER` | `none` | STT provider (`openai`) |
| `KNOW_STT_MODEL` | `gpt-4o-transcribe` | STT model name |
| `KNOW_STT_BASE_URL` | _(none)_ | Custom STT API base URL |
| `KNOW_AUDIO_SEGMENT_SECONDS` | `60` | Max audio segment duration for chunking (max 80 for Gemini) |

### Versioning

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_VERSION_COALESCE_MINUTES` | `10` | Min minutes between version snapshots |
| `KNOW_VERSION_RETENTION` | `50` | Max versions per file |

### Blob Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_BLOB_STORE` | `fs` | Backend: `fs` (local) or `s3` |
| `KNOW_BLOB_DIR` | `/data/blobs` | Local blob storage directory (fs backend) |
| `KNOW_BLOB_S3_BUCKET` | _(none)_ | S3 bucket name |
| `KNOW_BLOB_S3_PREFIX` | `blobs` | S3 key prefix |
| `KNOW_BLOB_S3_ENDPOINT` | _(none)_ | S3-compatible endpoint URL |
| `KNOW_BLOB_S3_REGION` | `us-east-1` | S3 region |

## CLI Flags for `know import`

| Flag | Description |
|------|-------------|
| `--vault` | Target vault ID (required) |
| `-r, --recursive` | Recurse into subdirectories (default: false) |
| `--force` | Overwrite existing files if content hash differs |
| `--dry-run` | Preview without changes |
| `-l, --labels` | Comma-separated labels to apply |
| `-y, --yes` | Skip confirmation prompt |
| `--no-ignore` | Import all files, ignoring .gitignore rules and dotfile filtering |
| `--api-url` | REST API base URL (default: `KNOW_SERVER_URL` or `http://localhost:8484`) |
| `--token` | API bearer token (or `KNOW_TOKEN`) |
