# Wiki-Link Resolution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Foam-style wiki-link resolution with stem-based matching, auto-updating on moves, API metadata, and Textual-based rendering in iOS/Mac apps.

**Architecture:** Add a `stem` field to the `file` table for fast lookup. Rewrite the resolver to use stem matching with path-suffix disambiguation. Update file lifecycle (create/move/delete) to maintain link consistency. Expose resolved wiki-link metadata in the document API. Migrate iOS/Mac apps from MarkdownUI to Textual with native `[[wiki-link]]` parsing.

**Tech Stack:** Go, SurrealDB, Swift, SwiftUI, Textual (swift-textual)

**Spec:** `docs/superpowers/specs/2026-03-16-wiki-link-resolution-design.md`

---

## Chunk 1: Schema + Stem Helper + Model

### Task 1: Add `FilenameStem` helper in models package

**Files:**
- Modify: `internal/models/file.go`
- Create: `internal/models/stem_test.go`

`FilenameStem` lives in `internal/models` (not `internal/file`) because `internal/db` needs to call it and importing `internal/file` from `internal/db` would create a circular dependency.

- [ ] **Step 1: Write the failing tests**

```go
// internal/models/stem_test.go
package models

import "testing"

func TestFilenameStem(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/notes/vector-embeddings.md", "vector-embeddings"},
		{"/daily/2026-03-16.md", "2026-03-16"},
		{"/README.md", "readme"},
		{"/notes/My Notes.txt", "my notes"},
		{"/a/b/c.md", "c"},
		{"/folder/", ""},
		{"file.md", "file"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := FilenameStem(tt.path)
			if got != tt.want {
				t.Errorf("FilenameStem(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just test 2>&1 | grep -A2 'TestFilenameStem'`
Expected: FAIL — `FilenameStem` not defined

- [ ] **Step 3: Write the implementation**

Add to `internal/models/file.go`:

```go
// FilenameStem returns the lowercase filename without extension from a path.
// e.g. "/notes/Vector-Embeddings.md" -> "vector-embeddings"
// Folders (paths ending with "/") return "".
func FilenameStem(path string) string {
	if strings.HasSuffix(path, "/") {
		return ""
	}
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.ToLower(stem)
}
```

Add imports: `"path/filepath"`, `"strings"` (may already be imported).

- [ ] **Step 4: Run test to verify it passes**

Run: `just test 2>&1 | grep -A2 'TestFilenameStem'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/file.go internal/models/stem_test.go
git commit -m "feat(wikilink): add FilenameStem helper"
```

### Task 2: Add `stem` field to schema and model

**Files:**
- Modify: `internal/db/schema.go`
- Modify: `internal/models/file.go`

- [ ] **Step 1: Add stem field to schema**

In `internal/db/schema.go`, add after `updated_at` field definition on `file`:

```sql
DEFINE FIELD IF NOT EXISTS stem ON file TYPE string DEFAULT "";
```

Add after existing file indexes:

```sql
DEFINE INDEX IF NOT EXISTS idx_file_vault_stem ON file FIELDS vault, stem;
```

- [ ] **Step 2: Add Stem field to File struct**

In `internal/models/file.go`, add to `File` struct after `Title`:

```go
Stem           string                 `json:"stem"`
```

- [ ] **Step 3: Run tests**

Run: `just test`
Expected: All pass (stem defaults to "")

- [ ] **Step 4: Commit**

```bash
git add internal/db/schema.go internal/models/file.go
git commit -m "feat(wikilink): add stem field to file schema and model"
```

### Task 3: Set stem in CreateFile, UpdateFile, MoveFile, MoveFilesByPrefix

**Files:**
- Modify: `internal/db/queries_file.go`

- [ ] **Step 1: Update CreateFile to set stem**

In `CreateFile` (line ~141), add `stem = $stem` to the CREATE SQL. Add param:

```go
"stem": models.FilenameStem(input.Path),
```

For folders, stem will be "" because `FilenameStem` returns "" for folder-like paths. But `input.IsFolder` paths may not end with `/`. Add explicit check:

```go
stem := ""
if !input.IsFolder {
	stem = models.FilenameStem(input.Path)
}
```

- [ ] **Step 2: Update UpdateFile to set stem**

In `UpdateFile` (line ~228), add `stem = $stem` to the UPDATE SQL. Add param:

```go
"stem": models.FilenameStem(input.Path),
```

Same folder guard as CreateFile.

- [ ] **Step 3: Update MoveFile to set stem**

In `MoveFile` (line ~307), update SQL:

```sql
UPDATE type::record("file", $id) SET
    path = $new_path,
    stem = $stem
RETURN AFTER
```

Add param: `"stem": models.FilenameStem(newPath)`

- [ ] **Step 4: Update MoveFilesByPrefix to recompute stems**

In `MoveFilesByPrefix` (line ~284), add `stem` recomputation to the SQL. Since SurrealQL can't easily replicate `FilenameStem` for all extensions, use a two-step approach: move paths first (existing SQL), then update stems in a second query:

```sql
-- After the existing path move query, add:
UPDATE file SET stem = string::lowercase(
    string::reverse(string::split(string::reverse(
        string::replace(path, "." + array::last(string::split(path, ".")), "")
    ), "/")[0])
)
WHERE vault = type::record("vault", $vault_id)
    AND string::starts_with(path, $new_prefix)
    AND is_folder = false
```

Actually, this SurrealQL is too fragile. Better approach: after `MoveFilesByPrefix` returns, have the caller (in `service.go`) fetch affected files and update stems in Go. Add a `TODO` comment in `MoveFilesByPrefix` noting stems need to be recomputed by the caller.

Simplest approach for now: add a `RecomputeStems` method:

```go
// RecomputeStems updates the stem field for all non-folder files matching a path prefix.
func (c *Client) RecomputeStems(ctx context.Context, vaultID, pathPrefix string) error {
	defer c.logOp(ctx, "file.recompute_stems", time.Now())
	sql := `SELECT id, path FROM file WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) AND is_folder = false`
	results, err := surrealdb.Query[[]struct {
		ID   surrealmodels.RecordID `json:"id"`
		Path string                 `json:"path"`
	}](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return fmt.Errorf("list files for stem recompute: %w", err)
	}
	rows := allResults(results)
	for _, row := range rows {
		id, err := models.RecordIDString(row.ID)
		if err != nil {
			continue
		}
		stem := models.FilenameStem(row.Path)
		sql := `UPDATE type::record("file", $id) SET stem = $stem`
		if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
			"id":   id,
			"stem": stem,
		}); err != nil {
			return fmt.Errorf("update stem for %s: %w", row.Path, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `just test`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries_file.go
git commit -m "feat(wikilink): set stem on file create/update/move"
```

---

## Chunk 2: New Resolver

### Task 4: Rewrite LinkResolver with stem-based algorithm

**Files:**
- Modify: `internal/file/links.go`

- [ ] **Step 1: Write the new Resolve method**

Replace the `Resolve` method in `internal/file/links.go`:

```go
// Resolve resolves a wiki-link target within a vault using Foam-style resolution.
// Resolution order:
//  1. Normalize: strip .md extension, extract stem (lowercase last segment)
//  2. Stem match: if exactly one file has that stem -> resolved
//  3. Path-suffix disambiguation: if target has path segments, filter by suffix
//
// Returns nil if not found or ambiguous.
func (r *LinkResolver) Resolve(ctx context.Context, vaultID, target string) (*models.File, error) {
	normalized := strings.TrimSuffix(target, ".md")
	if normalized == "" {
		return nil, nil
	}

	stem := models.FilenameStem("/" + normalized + ".md")
	if stem == "" {
		return nil, nil
	}

	// Stem match
	sql := `SELECT * FROM file WHERE is_folder = false AND vault = type::record("vault", $vault_id) AND stem = $stem`
	results, err := surrealdb.Query[[]models.File](ctx, r.db.DB(), sql, map[string]any{
		"vault_id": vaultID,
		"stem":     stem,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve link (stem): %w", err)
	}
	var matches []models.File
	if results != nil && len(*results) > 0 {
		matches = (*results)[0].Result
	}
	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	// Path-suffix disambiguation (only if target has path segments)
	if !strings.Contains(normalized, "/") {
		return nil, nil // ambiguous, no path segments to disambiguate
	}
	suffix := "/" + strings.ToLower(normalized) + ".md"
	var matched *models.File
	for i := range matches {
		if strings.HasSuffix(strings.ToLower(matches[i].Path), suffix) {
			if matched != nil {
				return nil, nil // still ambiguous
			}
			matched = &matches[i]
		}
	}
	return matched, nil
}
```

Add `"strings"` to imports.

- [ ] **Step 2: Run existing tests**

Run: `just test`
Expected: All pass (existing integration tests should still work)

- [ ] **Step 3: Commit**

```bash
git add internal/file/links.go
git commit -m "feat(wikilink): rewrite resolver with Foam-style stem matching"
```

---

## Chunk 3: DB Queries for Lifecycle Events

### Task 5: Add stem-based DB queries

**Files:**
- Modify: `internal/db/queries_file.go`
- Modify: `internal/db/queries_wikilink.go`

- [ ] **Step 1: Add GetFilesByStem**

In `internal/db/queries_file.go`:

```go
// GetFilesByStem returns non-folder files matching a stem in a vault.
func (c *Client) GetFilesByStem(ctx context.Context, vaultID, stem string) ([]models.File, error) {
	defer c.logOp(ctx, "file.get_by_stem", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND stem = $stem AND is_folder = false`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return nil, fmt.Errorf("get files by stem: %w", err)
	}
	return allResults(results), nil
}

// CountFilesByStem returns the number of non-folder files with a given stem in a vault.
func (c *Client) CountFilesByStem(ctx context.Context, vaultID, stem string) (int, error) {
	defer c.logOp(ctx, "file.count_by_stem", time.Now())
	sql := `SELECT count() AS total FROM file WHERE vault = type::record("vault", $vault_id) AND stem = $stem AND is_folder = false GROUP ALL`
	results, err := surrealdb.Query[[]struct{ Total int `json:"total"` }](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return 0, fmt.Errorf("count files by stem: %w", err)
	}
	rows := allResults(results)
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}

// ListFilesByPrefix returns non-folder files whose path starts with prefix.
func (c *Client) ListFilesByPrefix(ctx context.Context, vaultID, prefix string) ([]models.File, error) {
	defer c.logOp(ctx, "file.list_by_prefix", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) AND is_folder = false`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list files by prefix: %w", err)
	}
	return allResults(results), nil
}
```

- [ ] **Step 2: Add ResolveDanglingLinksByStem**

Uses SurrealQL filtering instead of fetching all dangling links. In `internal/db/queries_wikilink.go`:

```go
// ResolveDanglingLinksByStem resolves dangling wiki-links in a vault whose
// raw_target (normalized to a stem) matches the given stem.
// Caller must verify the stem is unambiguous before calling.
func (c *Client) ResolveDanglingLinksByStem(ctx context.Context, vaultID, stem, toFileID string) (int, error) {
	defer c.logOp(ctx, "wikilink.resolve_dangling_by_stem", time.Now())
	// Match dangling links where raw_target is stem-only (no "/") and lowercased
	// matches the stem. Also match targets ending with ".md" after stripping.
	sql := `
		UPDATE wiki_link SET to_file = type::record("file", $to_file_id)
		WHERE vault = type::record("vault", $vault_id)
			AND to_file IS NONE
			AND raw_target NOT CONTAINS "/"
			AND (
				string::lowercase(raw_target) = $stem
				OR string::lowercase(string::replace(raw_target, ".md", "")) = $stem
			)
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"stem":       stem,
		"to_file_id": toFileID,
	})
	if err != nil {
		return 0, fmt.Errorf("resolve dangling links by stem: %w", err)
	}
	return countResults(results), nil
}
```

- [ ] **Step 3: Add UnresolveStemOnlyLinks**

Uses SurrealQL filtering. In `internal/db/queries_wikilink.go`:

```go
// UnresolveStemOnlyLinks sets to_file = NONE on resolved wiki-links whose
// raw_target is stem-only (no path segments) and matches the given stem.
// Used when a new stem collision makes stem-only links ambiguous.
func (c *Client) UnresolveStemOnlyLinks(ctx context.Context, vaultID, stem string) (int, error) {
	defer c.logOp(ctx, "wikilink.unresolve_stem_only", time.Now())
	sql := `
		UPDATE wiki_link SET to_file = NONE
		WHERE vault = type::record("vault", $vault_id)
			AND to_file IS NOT NONE
			AND raw_target NOT CONTAINS "/"
			AND (
				string::lowercase(raw_target) = $stem
				OR string::lowercase(string::replace(raw_target, ".md", "")) = $stem
			)
	`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return 0, fmt.Errorf("unresolve stem-only links: %w", err)
	}
	return countResults(results), nil
}
```

- [ ] **Step 4: Add GetWikiLinksToFile and UpdateWikiLinkRawTarget**

In `internal/db/queries_wikilink.go`:

```go
// GetWikiLinksToFile returns all wiki-links pointing to a given file.
func (c *Client) GetWikiLinksToFile(ctx context.Context, fileID string) ([]models.WikiLink, error) {
	defer c.logOp(ctx, "wikilink.get_to_file", time.Now())
	sql := `SELECT * FROM wiki_link WHERE to_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]models.WikiLink](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links to file: %w", err)
	}
	return allResults(results), nil
}

// UpdateWikiLinkRawTarget updates the raw_target of a single wiki-link.
func (c *Client) UpdateWikiLinkRawTarget(ctx context.Context, linkID, newTarget string) error {
	defer c.logOp(ctx, "wikilink.update_raw_target", time.Now())
	sql := `UPDATE type::record("wiki_link", $link_id) SET raw_target = $new_target`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"link_id":    bareID("wiki_link", linkID),
		"new_target": newTarget,
	}); err != nil {
		return fmt.Errorf("update wiki link raw target: %w", err)
	}
	return nil
}

// GetWikiLinksWithTargetInfo returns outgoing wiki-links from a file,
// with resolved target path and title via record link traversal.
type WikiLinkWithTarget struct {
	RawTarget string  `json:"raw_target"`
	Path      *string `json:"path"`
	Title     *string `json:"title"`
}

func (c *Client) GetWikiLinksWithTargetInfo(ctx context.Context, fromFileID string) ([]WikiLinkWithTarget, error) {
	defer c.logOp(ctx, "wikilink.get_with_target_info", time.Now())
	sql := `SELECT raw_target, to_file.path AS path, to_file.title AS title FROM wiki_link WHERE from_file = type::record("file", $file_id)`
	results, err := surrealdb.Query[[]WikiLinkWithTarget](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fromFileID),
	})
	if err != nil {
		return nil, fmt.Errorf("get wiki links with target info: %w", err)
	}
	return allResults(results), nil
}
```

- [ ] **Step 5: Run tests**

Run: `just test`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries_file.go internal/db/queries_wikilink.go
git commit -m "feat(wikilink): add stem-based DB queries for lifecycle events"
```

---

## Chunk 4: Update File Lifecycle (Create, Move, Delete)

### Task 6: Add ShortestUnambiguousTarget helper

**Files:**
- Modify: `internal/file/links.go`
- Modify: `internal/file/links_test.go` (create if not exists)

- [ ] **Step 1: Write tests**

```go
// internal/file/links_test.go
package file

import (
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestShortestUnambiguousTarget(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		others []models.File
		want   string
	}{
		{
			name:   "unique stem",
			path:   "/notes/vector-embeddings.md",
			others: []models.File{{Path: "/notes/vector-embeddings.md"}},
			want:   "vector-embeddings",
		},
		{
			name: "two files same stem",
			path: "/a/notes.md",
			others: []models.File{
				{Path: "/a/notes.md"},
				{Path: "/b/notes.md"},
			},
			want: "a/notes",
		},
		{
			name: "deeper disambiguation needed",
			path: "/x/a/notes.md",
			others: []models.File{
				{Path: "/x/a/notes.md"},
				{Path: "/y/a/notes.md"},
			},
			want: "x/a/notes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortestUnambiguousTarget(tt.path, tt.others)
			if got != tt.want {
				t.Errorf("ShortestUnambiguousTarget(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Write implementation**

In `internal/file/links.go`:

```go
// ShortestUnambiguousTarget computes the shortest wiki-link target that
// unambiguously identifies a file given all files sharing its stem.
// If stem is unique, returns the stem. Otherwise adds minimum path segments.
func ShortestUnambiguousTarget(filePath string, allWithSameStem []models.File) string {
	stem := models.FilenameStem(filePath)
	if len(allWithSameStem) <= 1 {
		return stem
	}

	parts := strings.Split(strings.TrimPrefix(filePath, "/"), "/")
	// Remove extension from last part
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))

	for depth := 1; depth < len(parts); depth++ {
		candidate := strings.Join(parts[len(parts)-depth-1:], "/")
		candidateLower := strings.ToLower(candidate)
		suffix := "/" + candidateLower + ".md"

		matchCount := 0
		for _, f := range allWithSameStem {
			if strings.HasSuffix(strings.ToLower(f.Path), suffix) {
				matchCount++
			}
		}
		if matchCount == 1 {
			return strings.ToLower(candidate)
		}
	}

	full := strings.TrimPrefix(filePath, "/")
	full = strings.TrimSuffix(full, filepath.Ext(full))
	return strings.ToLower(full)
}
```

Add imports: `"path/filepath"`, `"strings"`.

- [ ] **Step 3: Run tests**

Run: `just test 2>&1 | grep -A2 'TestShortest'`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/file/links.go internal/file/links_test.go
git commit -m "feat(wikilink): add ShortestUnambiguousTarget helper"
```

### Task 7: Rewrite resolveDanglingForFile and add handleStemAmbiguity

**Files:**
- Modify: `internal/file/service.go`

- [ ] **Step 1: Rewrite resolveDanglingForFile**

Replace the method in `internal/file/service.go`:

```go
func (s *Service) resolveDanglingForFile(ctx context.Context, vaultID string, doc *models.File) error {
	logger := logutil.FromCtx(ctx)

	if doc.Stem == "" {
		return nil
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract file id: %w", err)
	}

	// Only resolve if this stem is unambiguous (exactly one file with this stem)
	count, err := s.db.CountFilesByStem(ctx, vaultID, doc.Stem)
	if err != nil {
		return fmt.Errorf("count files by stem: %w", err)
	}
	if count != 1 {
		return nil // ambiguous or unexpected
	}

	n, err := s.db.ResolveDanglingLinksByStem(ctx, vaultID, doc.Stem, fileID)
	if err != nil {
		return fmt.Errorf("resolve dangling by stem: %w", err)
	}
	if n > 0 {
		logger.Info("resolved dangling wiki-links by stem", "stem", doc.Stem, "count", n)
	}

	return nil
}
```

- [ ] **Step 2: Add handleStemAmbiguity**

Add to `internal/file/service.go`:

```go
// handleStemAmbiguity checks if a file's stem collides with existing files
// and un-resolves wiki-links that are now ambiguous.
func (s *Service) handleStemAmbiguity(ctx context.Context, vaultID, stem string) error {
	if stem == "" {
		return nil
	}
	logger := logutil.FromCtx(ctx)

	count, err := s.db.CountFilesByStem(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("count files by stem: %w", err)
	}
	if count <= 1 {
		return nil
	}

	n, err := s.db.UnresolveStemOnlyLinks(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("unresolve ambiguous stem links: %w", err)
	}
	if n > 0 {
		logger.Info("un-resolved ambiguous stem-only wiki-links", "stem", stem, "count", n)
	}
	return nil
}
```

- [ ] **Step 3: Wire handleStemAmbiguity into file creation**

Find calls to `resolveDanglingForFile` in `service.go` and `handlers.go`. After each call, add:

```go
if err := svc.handleStemAmbiguity(ctx, vaultID, doc.Stem); err != nil {
    return fmt.Errorf("handle stem ambiguity for %s: %w", doc.Path, err)
}
```

Check exact locations: `service.go` around line 392, `handlers.go` around line 60.

- [ ] **Step 4: Add recomputeIncomingRawTargets helper**

Add to `internal/file/service.go`:

```go
// recomputeIncomingRawTargets updates raw_target on all wiki-links pointing
// to the given file to the shortest unambiguous form.
func (s *Service) recomputeIncomingRawTargets(ctx context.Context, vaultID, fileID, filePath string) error {
	logger := logutil.FromCtx(ctx)

	stem := models.FilenameStem(filePath)
	if stem == "" {
		return nil
	}

	sameStems, err := s.db.GetFilesByStem(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("get files by stem: %w", err)
	}

	newTarget := ShortestUnambiguousTarget(filePath, sameStems)

	links, err := s.db.GetWikiLinksToFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("get wiki links to file: %w", err)
	}

	for _, link := range links {
		if link.RawTarget == newTarget {
			continue
		}
		linkID, err := models.RecordIDString(link.ID)
		if err != nil {
			logger.Warn("failed to extract link ID", "error", err)
			continue
		}
		if err := s.db.UpdateWikiLinkRawTarget(ctx, linkID, newTarget); err != nil {
			return fmt.Errorf("update raw target for link %s: %w", linkID, err)
		}
	}

	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `just test`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/file/service.go internal/file/handlers.go
git commit -m "feat(wikilink): stem-based dangling resolution and ambiguity handling"
```

### Task 8: Rewrite Move and MoveByPrefix

**Files:**
- Modify: `internal/file/service.go`

- [ ] **Step 1: Rewrite Move method**

Replace the `Move` method in `internal/file/service.go`:

```go
func (s *Service) Move(ctx context.Context, vaultID, oldPath, newPath string) (*models.File, error) {
	oldPath = models.NormalizePath(oldPath)
	doc, err := s.db.GetFileByPath(ctx, vaultID, oldPath)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("file not found: %s", oldPath)
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract file id: %w", err)
	}

	normalizedNew := models.NormalizePath(newPath)
	oldStem := models.FilenameStem(oldPath)
	doc, err = s.db.MoveFile(ctx, fileID, normalizedNew)
	if err != nil {
		return nil, fmt.Errorf("move file: %w", err)
	}

	// Recompute raw_targets for wiki-links pointing to this file
	if err := s.recomputeIncomingRawTargets(ctx, vaultID, fileID, normalizedNew); err != nil {
		return nil, fmt.Errorf("recompute incoming raw targets: %w", err)
	}

	// Handle ambiguity at the new stem
	newStem := models.FilenameStem(normalizedNew)
	if err := s.handleStemAmbiguity(ctx, vaultID, newStem); err != nil {
		return nil, fmt.Errorf("handle stem ambiguity (new): %w", err)
	}

	// If stem changed, old stem may now be unambiguous — re-resolve dangling links
	if oldStem != newStem && oldStem != "" {
		count, err := s.db.CountFilesByStem(ctx, vaultID, oldStem)
		if err != nil {
			logutil.FromCtx(ctx).Warn("count files by old stem after move", "error", err)
		} else if count == 1 {
			remaining, err := s.db.GetFilesByStem(ctx, vaultID, oldStem)
			if err == nil && len(remaining) == 1 {
				remainingID, err := models.RecordIDString(remaining[0].ID)
				if err == nil {
					if n, err := s.db.ResolveDanglingLinksByStem(ctx, vaultID, oldStem, remainingID); err != nil {
						logutil.FromCtx(ctx).Warn("resolve dangling after move", "error", err)
					} else if n > 0 {
						logutil.FromCtx(ctx).Info("resolved dangling links after stem collision removal", "stem", oldStem, "count", n)
					}
				}
			}
		}
	}

	// Ensure destination folders exist
	if err := s.db.EnsureFolders(ctx, vaultID, normalizedNew); err != nil {
		return nil, fmt.Errorf("ensure destination folders: %w", err)
	}

	s.publishFileMoveEvent(vaultID, doc, oldPath)

	return doc, nil
}
```

- [ ] **Step 2: Update MoveByPrefix**

In the `MoveByPrefix` method, after moving files and folders:
1. Call `s.db.RecomputeStems(ctx, vaultID, newNorm)` to fix stems
2. Remove the call to `s.db.UpdateWikiLinkRawTargetsByPrefix()`
3. Fetch affected files and call `recomputeIncomingRawTargets` per file:

```go
// After moving files, recompute stems
if err := s.db.RecomputeStems(ctx, vaultID, newNorm); err != nil {
	return moved, fmt.Errorf("recompute stems: %w", err)
}

// Recompute raw_targets for affected files
affectedFiles, err := s.db.ListFilesByPrefix(ctx, vaultID, newNorm)
if err != nil {
	return moved, fmt.Errorf("list affected files: %w", err)
}
for _, f := range affectedFiles {
	fID, err := models.RecordIDString(f.ID)
	if err != nil {
		continue
	}
	if err := s.recomputeIncomingRawTargets(ctx, vaultID, fID, f.Path); err != nil {
		logutil.FromCtx(ctx).Warn("recompute raw targets after prefix move", "path", f.Path, "error", err)
	}
}
```

- [ ] **Step 3: Update file delete for stem re-resolution**

In the delete method (around line 727 in `service.go`), after `UnresolveWikiLinksToFile`, add:

```go
// If deletion removes a stem collision, re-resolve dangling links
stem := models.FilenameStem(doc.Path)
if stem != "" {
	count, err := s.db.CountFilesByStem(ctx, vaultID, stem)
	if err != nil {
		logutil.FromCtx(ctx).Warn("count files by stem after delete", "error", err)
	} else if count == 1 {
		remaining, err := s.db.GetFilesByStem(ctx, vaultID, stem)
		if err == nil && len(remaining) == 1 {
			remainingID, err := models.RecordIDString(remaining[0].ID)
			if err == nil {
				if n, err := s.db.ResolveDanglingLinksByStem(ctx, vaultID, stem, remainingID); err != nil {
					logutil.FromCtx(ctx).Warn("resolve dangling after delete", "error", err)
				} else if n > 0 {
					logutil.FromCtx(ctx).Info("resolved dangling links after stem collision removal", "stem", stem, "count", n)
				}
			}
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `just test`
Expected: All pass (update existing move/delete tests if they assert on old behavior)

- [ ] **Step 5: Commit**

```bash
git add internal/file/service.go
git commit -m "feat(wikilink): rewrite Move/MoveByPrefix/Delete for stem-based resolution"
```

---

## Chunk 5: API Changes

### Task 9: Add wikiLinks to Document API response

**Files:**
- Modify: `internal/api/types.go`
- Modify: `internal/api/files.go`

- [ ] **Step 1: Add WikiLinkInfo type and field**

In `internal/api/types.go`, add:

```go
// WikiLinkInfo is a resolved wiki-link for the document API response.
type WikiLinkInfo struct {
	RawTarget string  `json:"rawTarget"`
	Path      *string `json:"path"`
	Title     *string `json:"title"`
}
```

Add to `Document` struct:

```go
WikiLinks []WikiLinkInfo `json:"wikiLinks,omitempty"`
```

- [ ] **Step 2: Populate wikiLinks in getDocument handler**

In `internal/api/files.go`, update `getDocument`. Before `writeJSON`, add:

```go
fileID, _ := models.RecordIDString(doc.ID)
wikiLinks, err := s.app.DBClient().GetWikiLinksWithTargetInfo(ctx, fileID)
if err != nil {
	logger.Warn("failed to get wiki links", "error", err)
}

resp := documentFromModel(doc)
if len(wikiLinks) > 0 {
	resp.WikiLinks = make([]WikiLinkInfo, len(wikiLinks))
	for i, wl := range wikiLinks {
		resp.WikiLinks[i] = WikiLinkInfo{
			RawTarget: wl.RawTarget,
			Path:      wl.Path,
			Title:     wl.Title,
		}
	}
}
writeJSON(w, http.StatusOK, resp)
```

Remove the existing `writeJSON(w, http.StatusOK, documentFromModel(doc))` line.

- [ ] **Step 3: Run tests**

Run: `just test`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add internal/api/types.go internal/api/files.go
git commit -m "feat(wikilink): add wikiLinks to document API response"
```

---

## Chunk 6: Integration Tests

### Task 10: Add integration tests for the full wiki-link lifecycle

**Files:**
- Modify: `internal/integration/lifecycle_test.go`

- [ ] **Step 1: Write stem resolution integration test**

Test that creating files with [[stem]] links resolves correctly. Test case-insensitivity.

- [ ] **Step 2: Write ambiguity integration test**

Test: create /a/notes.md, create source with [[notes]] (resolves), create /b/notes.md (becomes ambiguous), delete /b/notes.md (resolves again).

- [ ] **Step 3: Write move raw_target recomputation test**

Test: create file, create link, move file, verify raw_target is updated to shortest unambiguous form.

- [ ] **Step 4: Update existing move/delete wiki-link tests**

Update `TestDeleteUnresolvesIncomingWikiLinks`, `TestMoveUpdatesWikiLinkRawTargets`, `TestMoveByPrefixUpdatesWikiLinkRawTargets` to match new stem-based behavior.

- [ ] **Step 5: Run full test suite**

Run: `just test`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/integration/lifecycle_test.go
git commit -m "test(wikilink): add integration tests for stem resolution and ambiguity"
```

---

## Chunk 7: Cleanup + Docs

### Task 11: Remove old resolver code and unused queries

**Files:**
- Modify: `internal/db/queries_wikilink.go`

- [ ] **Step 1: Check which old queries are still used**

Search for usages of `UpdateWikiLinkRawTargets` and `UpdateWikiLinkRawTargetsByPrefix`. If no longer called from `Move`/`MoveByPrefix`, remove them.

Also check `ResolveDanglingLinks` (the old non-stem version) — if no longer called, remove.

- [ ] **Step 2: Remove unused queries and their tests**

- [ ] **Step 3: Run tests**

Run: `just test`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries_wikilink.go internal/db/queries_wikilink_test.go
git commit -m "refactor(wikilink): remove old path/title resolution queries"
```

### Task 12: Update feature docs

**Files:**
- Modify: `docs/feature-ingestion.md`

- [ ] **Step 1: Add wiki-link resolution section**

Document: Foam-style stem matching, case-insensitive, disambiguation with path segments, auto-updates on move, API wikiLinks field.

- [ ] **Step 2: Commit**

```bash
git add docs/feature-ingestion.md
git commit -m "docs: update wiki-link resolution documentation"
```

---

## Chunk 8: Swift Client — Textual Migration + Wiki-Link Rendering

### Task 13: Update Swift Document model

**Files:**
- Modify: `mac-app/ios/Models/Document.swift`

- [ ] **Step 1: Add WikiLinkInfo struct**

```swift
struct WikiLinkInfo: Codable {
    let rawTarget: String
    let path: String?
    let title: String?
}
```

- [ ] **Step 2: Add wikiLinks to Document**

Add field: `let wikiLinks: [WikiLinkInfo]`

Update `init(from decoder:)`:

```swift
wikiLinks = try container.decodeIfPresent([WikiLinkInfo].self, forKey: .wikiLinks) ?? []
```

- [ ] **Step 3: Build to verify**

Build the mac app project.

- [ ] **Step 4: Commit**

```bash
git add mac-app/ios/Models/Document.swift
git commit -m "feat(swift): add wikiLinks to Document model"
```

### Task 14: Replace MarkdownUI with Textual

**Files:**
- Modify: `mac-app/ios/project.yml`
- Modify: `mac-app/ios/Views/DocumentView.swift`

- [ ] **Step 1: Update project.yml**

Replace MarkdownUI package:

```yaml
packages:
  Textual:
    url: https://github.com/gonzalezreal/textual
    from: "0.1.0"
```

Update target dependency:

```yaml
dependencies:
  - package: Textual
```

- [ ] **Step 2: Update DocumentView.swift**

Replace `import MarkdownUI` with `import Textual`. Update rendering call. Check Textual's actual API — the view name may differ from MarkdownUI's `Markdown()`.

- [ ] **Step 3: Build and verify**

- [ ] **Step 4: Commit**

```bash
git add mac-app/ios/project.yml mac-app/ios/Views/DocumentView.swift
git commit -m "feat(swift): migrate from MarkdownUI to Textual"
```

### Task 15: Implement wiki-link rendering

**Files:**
- Create: `mac-app/ios/Rendering/WikiLinkParser.swift`
- Modify: `mac-app/ios/Views/DocumentView.swift`

- [ ] **Step 1: Create WikiLinkParser**

Implement a custom `MarkupParser` conformance that:
1. Regex-replaces `[[target]]` and `[[target|display]]` with standard markdown links
2. Uses the `wikiLinks` array to resolve targets to titled links
3. Broken links get a distinct visual style

Use `know://doc/path` URL scheme for navigation.

- [ ] **Step 2: Update DocumentView to use WikiLinkParser**

Pass wiki-links to the parser when rendering.

- [ ] **Step 3: Add URL handler for know:// scheme**

Handle `know://doc/...` taps to navigate to the target document.

- [ ] **Step 4: Build and test manually**

- [ ] **Step 5: Commit**

```bash
git add mac-app/ios/Rendering/WikiLinkParser.swift mac-app/ios/Views/DocumentView.swift
git commit -m "feat(swift): wiki-link rendering with Textual custom parser"
```
