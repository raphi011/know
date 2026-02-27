# MCP SurrealDB Server

An MCP (Model Context Protocol) server in Go that connects to a SurrealDB instance to persist knowledge between agent sessions.

## Purpose

This server enables AI agents to store and retrieve knowledge across sessions, providing a persistent memory layer using SurrealDB as the backend database.

## Tech Stack

- **Language**: Go
- **Protocol**: MCP (Model Context Protocol)
- **Database**: SurrealDB

## Building

Use `just` for all build and test commands:

```bash
just build      # Build CLI binary
just server     # Build server binary
just build-all  # Build both
just test       # Run tests
just dev        # Start full dev environment
```

**IMPORTANT**: Before committing any changes, always run `just test`.

**IMPORTANT**: Always use `just build` or `just build-all` instead of raw `go build ./...`. The justfile includes `-buildvcs=false` which is required because this project is in a subdirectory of the git repo. Raw `go build` will fail with `error obtaining VCS status: exit status 128`.

## SurrealDB Reference

For SurrealDB-specific syntax, v3.0 breaking changes, and query patterns:
- **Subagent**: Use the `surrealdb` subagent for complex query work (has built-in reference guide)

## Error Handling

**CRITICAL**: Never ignore errors with `_ =` assignments. All errors must be either:
1. Returned to the caller with context: `return fmt.Errorf("operation: %w", err)`
2. Logged with structured logging: `slog.Warn("operation failed", "key", value, "error", err)`
3. Explicitly justified with a comment explaining why it's safe to ignore

This applies to:
- Database operations (`CreateEntity`, `UpdateEntityAccess`, `GetEntityByName`, etc.)
- ID extraction (`models.RecordIDString`)
- Any function that returns an error

Silent failures make debugging impossible and degrade features without any indication.

## GraphQL Code Generation

After modifying `internal/graph/schema.graphqls`, regenerate the GraphQL code:

```bash
just generate
```

### Generation Tips

1. **Helper functions**: Conversion helpers (like `entityToGraphQL`, `serviceJobToGraphQL`) live in `internal/graph/helpers.go` - NOT in `schema.resolvers.go`. gqlgen will move any helper functions from resolvers to a commented block during regeneration.

2. **Generation order**: When adding new GraphQL types that require new helper functions:
   - First: Update `schema.graphqls` with new fields/types
   - Second: Add/update helpers in `helpers.go`
   - Third: Run `just generate`
   - Fourth: Update resolver code in `schema.resolvers.go` to use the new helpers
   - Fifth: Verify with `just build-all && just test`

## Documentation

**IMPORTANT**: When adding or modifying features, always update `README.md` with example prompts showcasing what the feature can do. This helps users understand how to use each tool effectively.

### Technical Learnings (`docs/`)

When learning something new about embeddings, SurrealDB, RAG, LLMs, or the tech stack:
1. Add learnings to the appropriate file in `docs/`
2. Keep entries concise and practical
3. Include code examples where helpful

Available docs:
- `docs/embeddings.md` - Vector embeddings, models, dimensions
- `docs/surrealdb.md` - SurrealDB patterns, HNSW indexes, v3 syntax
- `docs/rag.md` - RAG architecture, chunking, hybrid search
- `docs/llm.md` - LLM integration patterns
- `docs/langchaingo.md` - Go LLM library usage
- `docs/bedrock.md` - AWS Bedrock + Teleport setup

## Knowhow v2 — Vault-Based Architecture

v2 lives alongside v1 in `internal/v2/`. It replaces the flat entity model with vault-scoped documents, wiki-links, and auth.

### Building v2

```bash
just build-server-v2     # GraphQL server on :8485
just build-cli-v2        # CLI (scrape command, uses GraphQL API)
just build-bootstrap-v2  # One-time DB bootstrap script
just build-all           # All binaries (v1 + v2)
just generate-v2         # Regenerate gqlgen code for v2
```

### v2 Architecture

```
cmd/knowhow-server-v2/   # GraphQL server (port 8485)
cmd/knowhow-v2/          # CLI client (talks to server via GraphQL)
cmd/bootstrap-v2/        # One-time script: creates user + vault + token
internal/v2/
├── models/              # Data structs + helpers (RecordIDString, ContentHash)
├── db/                  # SurrealDB client, DDL, query functions
├── document/            # Document lifecycle: parse → embed → link → chunk
├── vault/               # Vault CRUD + virtual folder derivation
├── search/              # Hybrid BM25 + vector search with RRF fusion
├── template/            # Template CRUD
├── auth/                # Token auth middleware + AuthContext
├── graph/               # GraphQL schema, resolvers, gqlgen config
└── integration/         # Full lifecycle integration tests
```

### Key v2 Patterns

- **SurrealDB v3 strict mode**: `option<T>` fields require `surrealmodels.None` (not Go `nil`/`NULL`)
- **Embedder is optional**: `nil` embedder disables AI features gracefully
- **Auth**: Bearer token → SHA256 hash → DB lookup → vault-scoped access
- **GraphQL v2**: schema at `internal/v2/graph/schema.graphqls`, config at `gqlgen-v2.yml`
- **Wiki-link resolution**: exact path match first, then title match (shortest path wins)
- **CLI uses GraphQL API**: `cmd/knowhow-v2/` never connects directly to DB
- **Bootstrap connects directly to DB**: `cmd/bootstrap-v2/` is a one-time setup script

### Running v2

```bash
# 1. Start SurrealDB
just db-up

# 2. Bootstrap (creates user, vault, API token)
go run -buildvcs=false ./cmd/bootstrap-v2 --name "Admin"
# Prints token to stdout, vault ID to stderr

# 3. Start v2 server
just build-server-v2
KNOWHOW_V2_PORT=8485 ./bin/knowhow-server-v2

# 4. Scrape documents
KNOWHOW_V2_TOKEN=kh_... ./bin/knowhow-v2 scrape ./docs --vault <vault-id>
```

### v2 Tests

```bash
go test -v ./internal/v2/...  # All v2 tests (30 tests across 4 packages)
```

## Web UI (Svelte + Vite)

The `web/` directory contains a Svelte 5 SPA that serves as a document editor.

### Development

```bash
just web-dev    # Start Vite dev server on :5173
just web-build  # Build production bundle to web/dist/
```

The Vite dev server proxies `/query` to the Go API on `:8484`. In production, the built assets are embedded via `go:embed` in `web/embed.go`.

### Key Details

- **Svelte 5 runes**: Uses `$state`, `$derived`, `$effect` (not old `$:` syntax or stores)
- **CodeMirror 6**: Editor wrapped as `Editor.svelte` — `lineWrapping` is `EditorView.lineWrapping`
- **GraphQL client**: `graphql-request` (lightweight, not Apollo) — queries in `src/lib/graphql/queries.ts`
- **Build**: `just build-server` runs `web-build` first, then `go build` with embedded dist/

### File Structure

```
web/
├── embed.go                      # go:embed all:dist
├── dist/                         # Built assets (gitignored content, stub checked in)
├── src/
│   ├── App.svelte                # Root: layout + state + save logic
│   ├── app.css                   # Global styles (dark/light theme via CSS vars)
│   ├── main.ts                   # Svelte mount
│   └── lib/
│       ├── graphql/
│       │   ├── client.ts         # GraphQLClient → /query
│       │   └── queries.ts        # LIST_DOCUMENTS, GET_ENTITY, UPDATE_CONTENT
│       └── components/
│           ├── Sidebar.svelte    # Document list + search filter
│           ├── Editor.svelte     # CodeMirror wrapper
│           └── SaveStatus.svelte # Save indicator
├── package.json
├── vite.config.ts                # Proxy /query → :8484
└── tsconfig.json
```

## Bubbletea v2 TUI

This project uses **bubbletea v2** for terminal UIs. Use the `bubbletea` subagent for TUI implementation.

### Import Paths (v2)

```go
import (
    "charm.land/bubbles/v2/progress"
    tea "charm.land/bubbletea/v2"
    "github.com/charmbracelet/lipgloss"  // lipgloss stays at v1
)
```

### Key v2 API Changes

- `View()` returns `tea.View`, use `tea.NewView(content)` wrapper
- `tea.KeyMsg` → `tea.KeyPressMsg`
- `Init()` returns `tea.Cmd` only (not `(Model, Cmd)`)
