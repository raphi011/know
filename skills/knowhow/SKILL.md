---
name: knowhow
description: >-
  This skill should be used when the user asks to "search my notes",
  "find in knowledge base", "look up my docs", "save a memory",
  "remember this", "create a note", "edit my document", "browse my vault",
  "check my notes on", "what do I know about", "search knowhow",
  "list my folders", "list my labels", or when interacting with the
  knowhow MCP knowledge base for document search, creation, and editing.
version: 1.0.0
---

# Knowhow Knowledge Base

Personal RAG knowledge base — hybrid search, document management, memories. Tools prefixed `mcp__knowhow__`.

## Quick Patterns

### Search

```
search_documents(query="topic")
search_documents(query="topic", labels=["ops"], folder="/guides/")
```

### Browse

```
list_folders()  →  list_folder_contents(folder="/guides/")
list_labels()   →  search_documents(query="topic", labels=["ops"])
```

### Read

```
get_document(path="/guides/foo.md")
get_document(path="/guides/foo.md", sections=true)   # section outline for targeted edits
```

### Create

```
create_memory(title="TIL xyz", content="...", labels=["til"])
create_document(path="/guides/new-guide.md", content="# Guide\n\n...")
```

### Edit (full replace)

```
get_document(path) → extract Content-Hash →
edit_document(path, content="# Full new content\n...", expected_hash="<hash>")
```

### Edit (section — token-efficient)

```
get_document(path, sections=true) → pick heading →
edit_document_section(path, operation="replace", heading="Setup", content="new body")
```

## Tools

| Tool | Key Params | Notes |
|------|-----------|-------|
| search_documents | query, labels?, folder?, doc_type?, limit? | Hybrid BM25+vector; returns title+snippet |
| get_document | path, sections? | Returns content_body + Content-Hash |
| list_labels | — | All labels, cached 60s |
| list_folders | — | All folder paths, cached 60s |
| list_folder_contents | folder | Immediate children only |
| get_document_versions | path, limit? | Version history with hashes |
| create_memory | title, content, labels? | Path: /memories/YYYY-MM-DD-{slug}.md |
| create_document | path, content | Fails if path exists |
| edit_document | path, content, expected_hash? | Full replacement; fails if NOT exists |
| edit_document_section | path, operation, heading?, content?, ... | Token-efficient section edits |

## Section Edit Operations

| Operation | Behavior |
|-----------|----------|
| replace | Replace section body (DEEP — includes subsections) |
| delete | Remove section (SHALLOW — subsections preserved) |
| insert_after | New section after target heading |
| insert_before | New section before target heading |
| append | New section at end of document |

- `heading=""` targets preamble (content before first heading)
- `position` disambiguates duplicate headings (0-indexed, default 0)
- `new_heading` + `new_level` (1-6) required for insert/append operations

## Workflows

### Safe Edit (optimistic locking)

1. `get_document(path, sections=true)` — note Content-Hash + section outline
2. `edit_document_section(path, operation, heading, content, expected_hash=hash)`
3. If hash mismatch error → re-read and retry

### Discovery

1. `list_folders()` — understand vault structure
2. `list_folder_contents(folder)` — browse specific area
3. `search_documents(query)` — find by content
4. `get_document(path)` — read full document

## Gotchas

| Issue | Detail |
|-------|--------|
| Writes go to first vault only | Reads span all vaults; writes always target first accessible vault |
| create vs edit | create_document fails if exists; edit_document fails if NOT exists |
| edit_document = full replace | Send complete content; use edit_document_section for targeted changes |
| get_document returns content_body | Frontmatter not shown in response but IS preserved on section edits |
| Labels/folders cached 60s | May be stale after recent creates/deletes |
| Memory path collision | Same title + same date → same slug → create fails |
| replace is DEEP, delete is SHALLOW | replace includes subsections; delete preserves them |
| list_folder_contents not recursive | Immediate children only — use list_folders for full tree |
