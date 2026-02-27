# Dynamic Query Blocks — Design

## Goal

Allow markdown documents to contain live query blocks that resolve to lists or tables of matching documents at read time, similar to Obsidian's Dataview plugin.

## Syntax

Fenced code blocks with language `knowhow`:

````markdown
```knowhow
FROM /projects
WHERE labels CONTAIN "go"
SHOW title, labels, updated_at
SORT updated_at DESC
LIMIT 10
```
````

### DSL Keywords

| Keyword | Required | Description | Default |
|---------|----------|-------------|---------|
| `FROM <folder>` | No | Scope to folder path prefix | Entire vault |
| `WHERE <condition>` | No | Filter conditions (AND-combined) | No filter |
| `SHOW <fields>` | No | Columns to return | `title, path` |
| `SORT <field> ASC\|DESC` | No | Result ordering | `title ASC` |
| `LIMIT <n>` | No | Max results | 50 |

### WHERE Conditions

- `labels CONTAIN "x"` — document has label
- `type = "note"` — document type equals
- `title CONTAINS "x"` — title substring match

Multiple WHERE clauses are AND-combined.

### Format Detection

- `SHOW` omitted or 1-2 fields: LIST format (bullet list of links)
- `SHOW` with >2 fields: TABLE format (columnar)

### Available SHOW Fields

`title`, `path`, `labels`, `doc_type`, `created_at`, `updated_at`, `source`

## Architecture

### Approach: Lazy resolver on Document fetch

Query blocks are parsed from raw content when a Document is fetched via GraphQL. No storage of query results — purely computed at read time.

- Zero DB schema changes
- Always fresh results
- Simple implementation
- Cost: N sub-queries per document fetch when `queryBlocks` is requested

### Components

1. **Parser** (`internal/parser/queryblock.go`): `ExtractQueryBlocks(content string) []RawQueryBlock` — finds fenced `knowhow` blocks, parses DSL into structured queries.

2. **GraphQL types** — new types added to v2 schema: `QueryBlock`, `QueryResult`, `QueryFormat` enum. New field `queryBlocks` on `Document`.

3. **Document resolver** — lazy field resolver for `queryBlocks`: parse content → build DB filter → run query → map results.

### GraphQL Schema Additions

```graphql
enum QueryFormat { LIST TABLE }

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

type Document {
  # ... existing fields ...
  queryBlocks: [QueryBlock!]!
}
```

### Resolver Flow

1. Parse `obj.Content` for `knowhow` code blocks
2. For each block, parse DSL into filter struct
3. Build `ListDocumentsFilter` from parsed DSL
4. Run `db.ListDocuments(ctx, filter)`
5. Map results to `QueryResult`, extracting only SHOW fields
6. Return `[]QueryBlock` with results and format

### Error Handling

- Malformed DSL: `error` field set on QueryBlock, empty results
- No matching docs: empty results, no error
- DB error: `error` field set with message

## Out of Scope (YAGNI)

- Recursive query resolution (queries in queried docs)
- Aggregation (COUNT, SUM)
- Cross-vault queries
- EMBED (inline full doc content)
- Write-time evaluation or caching
