# Wiki-Link Resolution Design

Foam-style wiki-link resolution with auto-updating on file moves and native rendering in the iOS/Mac app.

## Problem

All 469 wiki-links in the second-brain vault are broken. The current resolver tries exact path match and title match, but wiki-links like `[[vector-embeddings]]` don't match either:
- Path is `/02-notes/programming/vector-embeddings.md` (not `vector-embeddings`)
- Title is `"Vector Embeddings"` (derived from H1 heading, different casing + spaces)

Additionally, wiki-links render as plain text in the iOS/Mac app — they should be tappable links showing the target document's title.

## Design Decisions

- **Foam-style resolution**: stem-based matching with path-segment disambiguation for collisions
- **Case-insensitive resolution**: stems are stored and compared lowercase. The original `raw_target` casing is preserved for display.
- **Ambiguous matches stay broken** rather than silently picking one
- **Auto-update `raw_target`** on file move to shortest unambiguous form
- **API returns resolved wiki-link metadata** so clients can render titled links
- **Textual** replaces MarkdownUI for markdown rendering with native `[[wiki-link]]` parsing
- The parser extracts only the target portion from `[[target|display text]]` syntax before passing it to the resolver (existing behavior in goldmark wikilink extension)

## Schema Changes

### New field on `file` table

```sql
DEFINE FIELD IF NOT EXISTS stem ON file TYPE string DEFAULT "";
DEFINE INDEX IF NOT EXISTS idx_file_vault_stem ON file FIELDS vault, stem;
```

`stem` = lowercase filename without extension, computed from `path` on every create/move/upsert. Only set on non-folder files — folders keep `stem = ""`, so the index stays effective without needing an `is_folder` filter.

Examples:
- `/02-notes/programming/vector-embeddings.md` -> `"vector-embeddings"`
- `/daily/2026-03-16.md` -> `"2026-03-16"`

## Resolution Algorithm

Given target string from `[[target]]`:

1. **Normalize target**: strip `.md` extension if present (so `[[notes/foo.md]]` and `[[notes/foo]]` resolve the same way). Extract the last path segment as the stem, lowercase it.
2. **Stem match**: query `WHERE stem = $stem AND vault = $vault AND is_folder = false`. If exactly one match -> resolved. If zero -> broken. If multiple -> step 3.
3. **Path-suffix disambiguation**: target contains path segments (e.g. `programming/vector-embeddings`). Lowercase the full target and filter stem matches by `string::ends_with(string::lowercase(path), "/" + $lowered_target + ".md")`. Must match exactly one -> resolved. Otherwise -> broken (ambiguous or no match).

### Resolution examples

| Target | Files in vault | Result |
|--------|---------------|--------|
| `[[vector-embeddings]]` | `/notes/vector-embeddings.md` (only one) | Resolved |
| `[[Vector-Embeddings]]` | `/notes/vector-embeddings.md` (only one) | Resolved (case-insensitive) |
| `[[vector-embeddings]]` | `/a/vector-embeddings.md`, `/b/vector-embeddings.md` | Broken (ambiguous) |
| `[[a/vector-embeddings]]` | `/a/vector-embeddings.md`, `/b/vector-embeddings.md` | Resolved (disambiguated) |
| `[[vector-embeddings.md]]` | `/notes/vector-embeddings.md` | Resolved (extension stripped) |
| `[[nonexistent]]` | (no match) | Broken |

## File Lifecycle Events

### On File Create/Upsert

1. Compute and store `stem` (only for non-folder files)
2. Resolve outgoing wiki-links using the new algorithm
3. Resolve dangling incoming links: find wiki-links where `to_file IS NONE` whose lowercased stem of `raw_target` matches the new file's stem. If unambiguous (only one file with that stem) -> resolve them.
4. **Ambiguity check**: if the new file's stem collides with an existing file, re-resolve all wiki-links whose `to_file` points to any file with the colliding stem. If the `raw_target` is stem-only (no path segments), set `to_file = NONE` (now ambiguous). If it has disambiguating path segments, re-run suffix match.

### On File Move (single file)

1. Update `path` and recompute `stem`
2. Find all wiki-links where `to_file` points to the moved file
3. Recompute each `raw_target` to the **shortest unambiguous form**:
   - If stem is unique in vault -> use stem only (e.g. `vector-embeddings`)
   - If stem is ambiguous -> use minimum path segments to disambiguate (e.g. `programming/vector-embeddings`)
4. If the move created or resolved stem collisions, handle ambiguity changes (same as create step 4)

### On Folder Move (MoveByPrefix)

1. Update paths and recompute stems for all affected files
2. For each affected file whose stem is unique in vault, no `raw_target` change needed (stem-only links still resolve)
3. For affected files with stem collisions, recompute `raw_target` to shortest unambiguous form per file
4. Handle any new or resolved ambiguity (same logic as single-file move step 4)

This is more expensive than the current prefix-replacement approach but ensures `raw_target` values are always in shortest-unambiguous form.

### On File Delete

1. Existing behavior: set `to_file = NONE` on incoming wiki-links
2. Attempt re-resolution of all dangling wiki-links matching the deleted file's stem. If exactly one file now matches that stem, resolve them. If zero, leave dangling. This handles the case where deletion removes a stem collision.

## API Change

Add `wikiLinks` to the `Document` response from `GET /api/documents`:

```json
{
  "id": "abc123",
  "path": "/notes/surreal-db.md",
  "title": "SurrealDB",
  "content": "...",
  "wikiLinks": [
    {
      "rawTarget": "vector-embeddings",
      "path": "/notes/programming/vector-embeddings.md",
      "title": "Vector Embeddings"
    },
    {
      "rawTarget": "nonexistent",
      "path": null,
      "title": null
    }
  ]
}
```

Each entry includes:
- `rawTarget`: the original text from `[[...]]`
- `path`: resolved file path (null if broken)
- `title`: resolved file title (null if broken)

### Query approach

Use SurrealDB record link traversal to fetch resolved file metadata in a single query:

```sql
SELECT raw_target, to_file.path AS path, to_file.title AS title
    FROM wiki_link WHERE from_file = type::record("file", $file_id)
```

## Client Rendering (Textual migration)

### Library migration

Replace MarkdownUI (maintenance mode) with [Textual](https://github.com/gonzalezreal/textual) in both iOS and Mac apps.

### Wiki-link parsing

Implement a custom `MarkupParser` conformance that:
- Parses standard markdown (delegates to Textual's built-in parser)
- Additionally recognizes `[[wiki-link]]` syntax as inline links
- Uses the `wikiLinks` array from the API to resolve each `[[target]]` to a titled, tappable link
- Unresolved links render visually distinct (e.g. dimmed or with a "broken link" style)

### Navigation

- Tapping a resolved wiki-link navigates to that document within the app
- Tapping a broken wiki-link does nothing (or shows a brief "not found" indicator)

## Removed from current resolver

The current resolver tries exact path match and title match as separate steps. These are removed in favor of the Foam-style stem-based resolution:
- **Exact path match**: subsumed by stem + suffix match. Targets with `.md` extension are handled by stripping the extension before resolution.
- **Title match**: titles come from H1 headings and don't reliably correspond to wiki-link targets. Foam doesn't use titles for resolution.

## Files to modify

### Backend (Go)
- `internal/db/schema.go` — add `stem` field + index
- `internal/models/file.go` — add `Stem` field to `File` struct
- `internal/db/queries_file.go` — set `stem` in CreateFile, UpsertFile, MoveFile, MoveFilesByPrefix
- `internal/file/links.go` — rewrite `LinkResolver.Resolve()` with new algorithm + add `FilenameStem()` helper
- `internal/file/service.go` — update `processWikiLinks`, `resolveDanglingForFile`, `processRelatesTo`, `Move`, `MoveByPrefix` for ambiguity handling and raw_target recomputation
- `internal/db/queries_wikilink.go` — add queries for stem-based dangling resolution and ambiguity un-resolution
- `internal/api/types.go` — add `WikiLinks` field to `Document`
- `internal/api/files.go` — populate `wikiLinks` in `getDocument` response using record link traversal query

### iOS/Mac (Swift)
- `project.yml` (both apps) — replace MarkdownUI with Textual dependency
- `Views/DocumentView.swift` (both apps) — use Textual renderer with custom parser
- `Models/Document.swift` — add `wikiLinks` to the model
- New file: custom `MarkupParser` implementation for wiki-link syntax
