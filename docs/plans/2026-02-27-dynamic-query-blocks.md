# Dynamic Query Blocks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add live query blocks (`knowhow` fenced code blocks) to markdown documents that resolve to document lists/tables at read time via a lazy GraphQL field resolver.

**Architecture:** A new parser extracts `knowhow` code blocks from markdown and parses a simple DSL (FROM/WHERE/SHOW/SORT/LIMIT). The GraphQL `Document` type gets a `queryBlocks` field resolver that parses content on-demand, runs DB queries via `ListDocuments`, and returns structured results. No DB schema changes — purely computed at read time.

**Tech Stack:** Go, gqlgen, existing `parser` + `v2/db` packages

---

### Task 1: Query Block Parser — Tests

**Files:**
- Create: `internal/parser/queryblock_test.go`

**Step 1: Write parser tests**

```go
// internal/parser/queryblock_test.go
package parser

import (
	"testing"
)

func TestExtractQueryBlocks_SingleBlock(t *testing.T) {
	content := "# My Doc\n\nSome text.\n\n```knowhow\nFROM /projects\nWHERE labels CONTAIN \"go\"\nSHOW title, labels, updated_at\nSORT updated_at DESC\nLIMIT 10\n```\n\nMore text."

	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	b := blocks[0]
	if b.Folder == nil || *b.Folder != "/projects" {
		t.Errorf("folder = %v, want /projects", b.Folder)
	}
	if len(b.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(b.Conditions))
	}
	if b.Conditions[0].Field != "labels" || b.Conditions[0].Op != OpContain || b.Conditions[0].Value != "go" {
		t.Errorf("condition = %+v", b.Conditions[0])
	}
	if len(b.ShowFields) != 3 || b.ShowFields[0] != "title" {
		t.Errorf("show = %v", b.ShowFields)
	}
	if b.SortField != "updated_at" || b.SortDesc != true {
		t.Errorf("sort = %s desc=%v", b.SortField, b.SortDesc)
	}
	if b.Limit != 10 {
		t.Errorf("limit = %d, want 10", b.Limit)
	}
}

func TestExtractQueryBlocks_NoBlocks(t *testing.T) {
	content := "# Just markdown\n\n```go\nfmt.Println(\"hello\")\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractQueryBlocks_MultipleBlocks(t *testing.T) {
	content := "```knowhow\nFROM /a\n```\n\ntext\n\n```knowhow\nWHERE type = \"note\"\nSHOW title, path, labels\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Folder == nil || *blocks[0].Folder != "/a" {
		t.Errorf("block 0 folder = %v", blocks[0].Folder)
	}
	if blocks[1].Folder != nil {
		t.Errorf("block 1 folder should be nil, got %v", blocks[1].Folder)
	}
}

func TestExtractQueryBlocks_DefaultValues(t *testing.T) {
	content := "```knowhow\nFROM /docs\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	// Defaults: SHOW title,path; SORT title ASC; LIMIT 50
	if len(b.ShowFields) != 2 || b.ShowFields[0] != "title" || b.ShowFields[1] != "path" {
		t.Errorf("default show = %v, want [title path]", b.ShowFields)
	}
	if b.SortField != "title" || b.SortDesc != false {
		t.Errorf("default sort = %s desc=%v", b.SortField, b.SortDesc)
	}
	if b.Limit != 50 {
		t.Errorf("default limit = %d, want 50", b.Limit)
	}
}

func TestExtractQueryBlocks_WhereConditions(t *testing.T) {
	content := "```knowhow\nWHERE labels CONTAIN \"go\"\nWHERE type = \"note\"\nWHERE title CONTAINS \"setup\"\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].Conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(blocks[0].Conditions))
	}
	c := blocks[0].Conditions
	if c[0].Op != OpContain {
		t.Errorf("c[0].Op = %v, want OpContain", c[0].Op)
	}
	if c[1].Op != OpEqual {
		t.Errorf("c[1].Op = %v, want OpEqual", c[1].Op)
	}
	if c[2].Op != OpContains {
		t.Errorf("c[2].Op = %v, want OpContains", c[2].Op)
	}
}

func TestExtractQueryBlocks_FormatDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    QueryFormat
	}{
		{"no SHOW = list", "```knowhow\nFROM /a\n```", FormatList},
		{"2 fields = list", "```knowhow\nSHOW title, path\n```", FormatList},
		{"3+ fields = table", "```knowhow\nSHOW title, path, labels\n```", FormatTable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ExtractQueryBlocks(tt.content)
			if len(blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(blocks))
			}
			if blocks[0].Format != tt.want {
				t.Errorf("format = %v, want %v", blocks[0].Format, tt.want)
			}
		})
	}
}

func TestExtractQueryBlocks_MalformedBlock(t *testing.T) {
	content := "```knowhow\nGARBAGE nonsense\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Error == "" {
		t.Error("expected error for malformed block")
	}
}

func TestExtractQueryBlocks_IndexTracking(t *testing.T) {
	content := "prefix\n\n```knowhow\nFROM /a\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	// Index should be byte offset of the opening ```
	expected := len("prefix\n\n")
	if blocks[0].Index != expected {
		t.Errorf("index = %d, want %d", blocks[0].Index, expected)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -buildvcs=false -v -run TestExtractQueryBlocks ./internal/parser/`
Expected: FAIL — `ExtractQueryBlocks` undefined

**Step 3: Commit**

```bash
git add internal/parser/queryblock_test.go
git commit -m "test: add query block parser tests

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Query Block Parser — Implementation

**Files:**
- Create: `internal/parser/queryblock.go`

**Step 1: Implement parser**

```go
// internal/parser/queryblock.go
package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// QueryFormat indicates how results should be rendered.
type QueryFormat int

const (
	FormatList  QueryFormat = iota // bullet list of links
	FormatTable                    // columnar table
)

// ConditionOp is a WHERE condition operator.
type ConditionOp int

const (
	OpContain  ConditionOp = iota // labels CONTAIN "x" (label membership)
	OpEqual                       // field = "x" (exact match)
	OpContains                    // field CONTAINS "x" (substring)
)

// Condition represents a parsed WHERE clause.
type Condition struct {
	Field string
	Op    ConditionOp
	Value string
}

// QueryBlock represents a parsed knowhow query block.
type QueryBlock struct {
	Index      int          // byte offset of opening ``` in content
	RawQuery   string       // raw DSL text inside the fences
	Folder     *string      // FROM folder
	Conditions []Condition  // WHERE clauses
	ShowFields []string     // SHOW columns
	SortField  string       // SORT field
	SortDesc   bool         // SORT DESC
	Limit      int          // LIMIT
	Format     QueryFormat  // derived from ShowFields count
	Error      string       // parse error, empty if ok
}

var (
	knowhowBlockRegex = regexp.MustCompile("(?s)```knowhow\\n(.*?)```")
	whereRegex        = regexp.MustCompile(`(?i)^WHERE\s+(.+)$`)
	fromRegex         = regexp.MustCompile(`(?i)^FROM\s+(\S+)$`)
	showRegex         = regexp.MustCompile(`(?i)^SHOW\s+(.+)$`)
	sortRegex         = regexp.MustCompile(`(?i)^SORT\s+(\S+)(?:\s+(ASC|DESC))?$`)
	limitRegex        = regexp.MustCompile(`(?i)^LIMIT\s+(\d+)$`)

	// WHERE condition patterns
	condContainRegex  = regexp.MustCompile(`(?i)^(\w+)\s+CONTAIN\s+"([^"]+)"$`)
	condEqualRegex    = regexp.MustCompile(`(?i)^(\w+)\s*=\s*"([^"]+)"$`)
	condContainsRegex = regexp.MustCompile(`(?i)^(\w+)\s+CONTAINS\s+"([^"]+)"$`)
)

// ExtractQueryBlocks finds all ```knowhow blocks in content and parses them.
func ExtractQueryBlocks(content string) []QueryBlock {
	matches := knowhowBlockRegex.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	blocks := make([]QueryBlock, 0, len(matches))
	for _, loc := range matches {
		rawQuery := content[loc[2]:loc[3]]
		block := parseQueryBlock(rawQuery)
		block.Index = loc[0]
		block.RawQuery = rawQuery
		blocks = append(blocks, block)
	}
	return blocks
}

func parseQueryBlock(raw string) QueryBlock {
	block := QueryBlock{
		ShowFields: []string{"title", "path"},
		SortField:  "title",
		SortDesc:   false,
		Limit:      50,
		Format:     FormatList,
	}

	hasShow := false
	hasValidLine := false
	lines := strings.Split(strings.TrimSpace(raw), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := fromRegex.FindStringSubmatch(line); m != nil {
			folder := m[1]
			block.Folder = &folder
			hasValidLine = true
		} else if m := whereRegex.FindStringSubmatch(line); m != nil {
			cond, err := parseCondition(m[1])
			if err != nil {
				block.Error = fmt.Sprintf("invalid WHERE: %s", err)
				return block
			}
			block.Conditions = append(block.Conditions, cond)
			hasValidLine = true
		} else if m := showRegex.FindStringSubmatch(line); m != nil {
			fields := strings.Split(m[1], ",")
			block.ShowFields = make([]string, 0, len(fields))
			for _, f := range fields {
				block.ShowFields = append(block.ShowFields, strings.TrimSpace(f))
			}
			hasShow = true
			hasValidLine = true
		} else if m := sortRegex.FindStringSubmatch(line); m != nil {
			block.SortField = strings.ToLower(m[1])
			block.SortDesc = strings.EqualFold(m[2], "DESC")
			hasValidLine = true
		} else if m := limitRegex.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n > 0 {
				block.Limit = n
			}
			hasValidLine = true
		} else {
			block.Error = fmt.Sprintf("unrecognized line: %s", line)
			return block
		}
	}

	if !hasValidLine {
		block.Error = "empty query block"
		return block
	}

	// Format detection: >2 SHOW fields = table
	if hasShow && len(block.ShowFields) > 2 {
		block.Format = FormatTable
	}

	return block
}

func parseCondition(raw string) (Condition, error) {
	if m := condContainRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpContain, Value: m[2]}, nil
	}
	if m := condEqualRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpEqual, Value: m[2]}, nil
	}
	if m := condContainsRegex.FindStringSubmatch(raw); m != nil {
		return Condition{Field: strings.ToLower(m[1]), Op: OpContains, Value: m[2]}, nil
	}
	return Condition{}, fmt.Errorf("cannot parse condition: %s", raw)
}
```

**Step 2: Run tests**

Run: `go test -buildvcs=false -v -run TestExtractQueryBlocks ./internal/parser/`
Expected: All PASS

**Step 3: Commit**

```bash
git add internal/parser/queryblock.go
git commit -m "feat: add query block parser for knowhow DSL

Parses FROM/WHERE/SHOW/SORT/LIMIT from fenced knowhow blocks.
Supports label contain, field equality, and substring conditions.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: GraphQL Schema + Types

**Files:**
- Modify: `internal/v2/graph/schema.graphqls`
- Modify: `internal/v2/graph/models.go`
- Modify: `gqlgen-v2.yml`

**Step 1: Add types to GraphQL schema**

Add before the `# INPUTS` section in `internal/v2/graph/schema.graphqls`:

```graphql
enum QueryFormat {
  LIST
  TABLE
}

type QueryBlock {
  index: Int!
  rawQuery: String!
  format: QueryFormat!
  results: [QueryResult!]!
  error: String
}

type QueryResult {
  docId: ID!
  title: String!
  path: String!
  fields: JSON
}
```

Add to the `Document` type (after `updatedAt`):

```graphql
  queryBlocks: [QueryBlock!]!
```

**Step 2: Add Go models to `internal/v2/graph/models.go`**

```go
type QueryBlock struct {
	Index    int            `json:"index"`
	RawQuery string         `json:"rawQuery"`
	Format   string         `json:"format"`
	Results  []QueryResult  `json:"results"`
	Error    *string        `json:"error,omitempty"`
}

type QueryResult struct {
	DocID  string         `json:"docId"`
	Title  string         `json:"title"`
	Path   string         `json:"path"`
	Fields map[string]any `json:"fields,omitempty"`
}
```

**Step 3: Add model mappings to `gqlgen-v2.yml`**

```yaml
  QueryBlock:
    model: github.com/raphaelgruber/memcp-go/internal/v2/graph.QueryBlock
  QueryResult:
    model: github.com/raphaelgruber/memcp-go/internal/v2/graph.QueryResult
  QueryFormat:
    model: github.com/raphaelgruber/memcp-go/internal/v2/graph.QueryFormat
```

Note: `QueryFormat` should be a string type in models.go — gqlgen will handle the enum mapping. Add to models.go:

```go
type QueryFormat string

const (
	QueryFormatList  QueryFormat = "LIST"
	QueryFormatTable QueryFormat = "TABLE"
)
```

**Step 4: Regenerate gqlgen**

Run: `go run github.com/99designs/gqlgen generate --config gqlgen-v2.yml`
Expected: Success — generates updated `generated.go` with new `QueryBlocks` resolver on `documentResolver`

**Step 5: Verify build**

Run: `go build -buildvcs=false ./internal/v2/graph/`
Expected: May fail with unimplemented resolver — that's expected, we implement in Task 4.

**Step 6: Commit**

```bash
git add internal/v2/graph/schema.graphqls internal/v2/graph/models.go gqlgen-v2.yml internal/v2/graph/generated.go internal/v2/graph/model_gen.go internal/v2/graph/schema.resolvers.go
git commit -m "feat(v2): add QueryBlock types to GraphQL schema

New types: QueryBlock, QueryResult, QueryFormat enum.
New field: Document.queryBlocks for lazy query resolution.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: QueryBlocks Resolver Implementation

**Files:**
- Modify: `internal/v2/graph/schema.resolvers.go`
- Modify: `internal/v2/graph/helpers.go`

**Step 1: Add query block execution helper to `helpers.go`**

```go
// internal/v2/graph/helpers.go — add these functions

func queryFormatToGraphQL(f parser.QueryFormat) QueryFormat {
	if f == parser.FormatTable {
		return QueryFormatTable
	}
	return QueryFormatList
}

// resolveQueryBlock executes a parsed query block against the database.
func resolveQueryBlock(ctx context.Context, dbClient *v2db.Client, vaultID string, parsed parser.QueryBlock) QueryBlock {
	block := QueryBlock{
		Index:    parsed.Index,
		RawQuery: parsed.RawQuery,
		Format:   queryFormatToGraphQL(parsed.Format),
		Results:  []QueryResult{},
	}

	if parsed.Error != "" {
		block.Error = &parsed.Error
		return block
	}

	// Build filter from parsed DSL
	filter := v2db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  parsed.Folder,
		Limit:   parsed.Limit,
	}

	// Map WHERE conditions to filter fields
	for _, cond := range parsed.Conditions {
		switch {
		case cond.Field == "labels" && cond.Op == parser.OpContain:
			filter.Labels = append(filter.Labels, cond.Value)
		case cond.Field == "type" && cond.Op == parser.OpEqual:
			filter.DocType = &cond.Value
		}
		// title CONTAINS handled in post-filter below
	}

	docs, err := dbClient.ListDocuments(ctx, filter)
	if err != nil {
		errMsg := fmt.Sprintf("query error: %v", err)
		block.Error = &errMsg
		return block
	}

	// Post-filter for conditions the DB filter doesn't support (title CONTAINS)
	var titleContains string
	for _, cond := range parsed.Conditions {
		if cond.Field == "title" && cond.Op == parser.OpContains {
			titleContains = strings.ToLower(cond.Value)
		}
	}

	for _, doc := range docs {
		if titleContains != "" && !strings.Contains(strings.ToLower(doc.Title), titleContains) {
			continue
		}
		docID, _ := models.RecordIDString(doc.ID)
		result := QueryResult{
			DocID: docID,
			Title: doc.Title,
			Path:  doc.Path,
		}
		// Build fields map for SHOW columns
		if len(parsed.ShowFields) > 0 {
			fields := make(map[string]any)
			for _, f := range parsed.ShowFields {
				switch f {
				case "title":
					fields["title"] = doc.Title
				case "path":
					fields["path"] = doc.Path
				case "labels":
					fields["labels"] = doc.Labels
				case "doc_type":
					fields["doc_type"] = doc.DocType
				case "created_at":
					fields["created_at"] = doc.CreatedAt
				case "updated_at":
					fields["updated_at"] = doc.UpdatedAt
				case "source":
					fields["source"] = doc.Source
				}
			}
			result.Fields = fields
		}
		block.Results = append(block.Results, result)
	}

	return block
}
```

**Step 2: Implement the QueryBlocks resolver in `schema.resolvers.go`**

Add to `schema.resolvers.go` (gqlgen will have added a stub):

```go
// QueryBlocks is the resolver for the queryBlocks field.
func (r *documentResolver) QueryBlocks(ctx context.Context, obj *Document) ([]*QueryBlock, error) {
	parsed := parser.ExtractQueryBlocks(obj.Content)
	if len(parsed) == 0 {
		return []*QueryBlock{}, nil
	}

	results := make([]*QueryBlock, len(parsed))
	for i, p := range parsed {
		block := resolveQueryBlock(ctx, r.db, obj.VaultID, p)
		results[i] = &block
	}
	return results, nil
}
```

The import block in `schema.resolvers.go` needs:
```go
import (
	"github.com/raphaelgruber/memcp-go/internal/parser"
)
```

The import block in `helpers.go` needs:
```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/parser"
	v2db "github.com/raphaelgruber/memcp-go/internal/v2/db"
	"github.com/raphaelgruber/memcp-go/internal/v2/models"
)
```

**Step 3: Verify build**

Run: `go build -buildvcs=false ./internal/v2/graph/`
Expected: Success

**Step 4: Commit**

```bash
git add internal/v2/graph/schema.resolvers.go internal/v2/graph/helpers.go
git commit -m "feat(v2): implement queryBlocks resolver

Parses knowhow DSL from document content at read time, runs
ListDocuments queries, returns structured results.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Integration Test — Query Blocks

**Files:**
- Modify: `internal/v2/db/db_test.go`

**Step 1: Add test for query block resolution via DB**

Add to `internal/v2/db/db_test.go`:

```go
func TestQueryBlockResolution(t *testing.T) {
	ctx := context.Background()
	vaultID := createTestVault(t, ctx)

	// Create documents with different labels and paths
	for _, doc := range []struct {
		path    string
		content string
		labels  []string
	}{
		{"/projects/go-app.md", "# Go App\nA Go project", []string{"go", "project"}},
		{"/projects/rust-app.md", "# Rust App\nA Rust project", []string{"rust", "project"}},
		{"/notes/setup.md", "# Setup Guide\nHow to set up", []string{"guide"}},
	} {
		_, _, err := testClient.UpsertDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        doc.path,
			Title:       doc.content[:strings.Index(doc.content, "\n")],
			Content:     doc.content,
			ContentBody: doc.content,
			Labels:      doc.labels,
			Source:      models.SourceManual,
		})
		if err != nil {
			t.Fatalf("create doc %s: %v", doc.path, err)
		}
	}

	// Query by folder
	docs, err := testClient.ListDocuments(ctx, ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  strPtr("/projects"),
	})
	if err != nil {
		t.Fatalf("list by folder: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs in /projects, got %d", len(docs))
	}

	// Query by label
	docs, err = testClient.ListDocuments(ctx, ListDocumentsFilter{
		VaultID: vaultID,
		Labels:  []string{"go"},
	})
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	if len(docs) != 1 {
		t.Errorf("expected 1 doc with label go, got %d", len(docs))
	}
}
```

Helper used above (add if not already present):
```go
func strPtr(s string) *string { return &s }
```

**Step 2: Run integration tests**

Run: `go test -buildvcs=false -v -count=1 -run TestQueryBlockResolution ./internal/v2/db/`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/v2/db/db_test.go
git commit -m "test(v2): add integration test for query block DB queries

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: Full Build Verification

**Step 1: Run all v2 tests**

Run: `go test -buildvcs=false -v -count=1 ./internal/v2/... ./internal/parser/`
Expected: All PASS

**Step 2: Build all binaries**

Run: `just build-all`
Expected: All 3 binaries compile

**Step 3: Final commit if any fixups needed**
