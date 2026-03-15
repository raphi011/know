# Document Ingestion Pipeline

The ingestion pipeline handles copying, parsing, embedding, and chunking documents into know vaults. It processes markdown files through a multi-stage pipeline that extracts metadata, resolves relations, and generates vector embeddings for semantic search.

## Overview

When a document is ingested via `know cp` or the WebDAV interface, it passes through the full document pipeline: **parse -> embed -> link -> chunk**. Unchanged files are automatically skipped based on content hash comparison, and every update creates an immutable version for rollback support.

## How It Works

### Pipeline Stages

1. **Parse** -- Extract frontmatter metadata (title, labels, summary, relations) and markdown content. Wiki-links and external URLs in the body are also detected during this phase.
2. **Embed** -- Generate vector embeddings for the document content using the configured embedding model.
3. **Link** -- Resolve `relates_to` entries from frontmatter and wiki-links in the body. Relations are stored as SurrealDB graph edges with a unique constraint on `(from, to, rel_type)`. Frontmatter relations are created automatically; wiki-link resolution uses exact path match first, then title match (shortest path wins). External URLs (both `[text](url)` markdown links and bare autolinked URLs) are stored in the `external_link` table with hostname, URL path, and source file reference.
4. **Chunk** -- Split the document into heading-based chunks for retrieval. Each heading section becomes its own chunk. Large sections exceeding the max size are split at paragraph boundaries. Empty sections are skipped.

### Chunking Strategy

Chunks use markdown-aware splitting (one chunk per heading). Before embedding, each chunk gets a context prefix prepended containing the document title and section path (contextual retrieval). Chunk metadata includes:

- `EntityID` -- Parent document reference
- `Content` -- The chunk text with context prefix
- `Position` -- Ordering within the document
- `HeadingPath` -- Full path of nested headings (e.g. `Overview > How It Works`)
- `Labels` -- Inherited from the document
- `Embedding` -- Vector embedding for semantic search

### Frontmatter

Documents support YAML frontmatter for structured metadata:

```yaml
---
type: document
title: Auth Service
labels: [work, infrastructure]
summary: Handles authentication and tokens
verified: true
relates_to:
  - user-service
  - john-doe
---
```

### Version History

Every document update creates a new version with a timestamp, source tag, and content hash. Versions are immutable -- rollback creates a new version with the old content and re-runs the full pipeline.

### Relations

Relations between documents can be created in three ways:

- **Frontmatter**: Add `relates_to` entries -- relations are created automatically during the pipeline.
- **Wiki-links**: Use `[[path/to/doc]]` syntax in markdown body -- resolved during parsing.

Relations require write access on the source vault and read access on the target vault. They are stored as SurrealDB graph edges with a unique constraint on `(from, to, rel_type)`.

### Query Blocks

Documents can embed live queries using an inline DSL inside `know` code blocks. These support `FROM`, `WHERE`, `SHOW`, `SORT`, and `LIMIT` clauses. Output format depends on the number of `SHOW` fields: 1-2 fields render as a list, 3+ fields render as a table.

## Usage

### Copying Files

```bash
# Copy top-level files (unchanged files are automatically skipped)
know cp ./docs / --vault default

# Recursive copy with labels
know cp ./notes /notes --vault default -r --labels "personal"

# Dry run (preview which files would be copied)
know cp ./wiki /wiki --vault default --dry-run

# Force overwrite files with different content hash
know cp ./docs /docs --vault default --force
```

Writing a file through the WebDAV interface also triggers the full pipeline on save.

## Reference

### CLI Flags for `know cp`

| Flag | Description |
|------|-------------|
| `--vault` | Target vault ID (required) |
| `-r, --recursive` | Recurse into subdirectories (default: false) |
| `--force` | Overwrite existing files if content hash differs |
| `--dry-run` | Preview without changes |
| `-l, --labels` | Comma-separated labels to apply |
| `--source` | Document source tag (default: `cp`) |
| `--api-url` | REST API base URL (default: `KNOW_SERVER_URL` or `http://localhost:8484`) |
| `--token` | API bearer token (or `KNOW_TOKEN`) |

### Environment Variables

- `KNOW_SERVER_URL` -- REST API base URL (alternative to `--api-url`)
- `KNOW_TOKEN` -- API bearer token (alternative to `--token`)
