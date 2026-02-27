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
- Database operations (`CreateDocument`, `CreateVault`, `GetTokenByHash`, etc.)
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

## Architecture — Vault-Based Document System

### Building

```bash
just build          # CLI binary
just build-server   # GraphQL server
just build-bootstrap # One-time DB bootstrap script
just build-all      # All binaries
just generate       # Regenerate gqlgen code
```

### Project Structure

```
cmd/knowhow-server/     # GraphQL server
cmd/knowhow/            # CLI client (scrape command, uses GraphQL API)
cmd/bootstrap/          # One-time script: creates user + vault + token
internal/
├── models/             # Data structs + helpers (RecordIDString, ContentHash)
├── db/                 # SurrealDB client, DDL, query functions, connection
├── document/           # Document lifecycle: parse → embed → link → chunk
├── vault/              # Vault CRUD + virtual folder derivation
├── search/             # Hybrid BM25 + vector search with RRF fusion
├── template/           # Template CRUD
├── auth/               # Token auth middleware + AuthContext
├── graph/              # GraphQL schema, resolvers, gqlgen config
├── parser/             # Markdown parsing, wiki-link extraction, chunking
├── llm/                # LLM/embedding provider abstraction
├── config/             # Configuration loading
├── metrics/            # Metrics collection
└── integration/        # Full lifecycle integration tests
```

### Key Patterns

- **SurrealDB v3 strict mode**: `option<T>` fields require `surrealmodels.None` (not Go `nil`/`NULL`)
- **Embedder is optional**: `nil` embedder disables AI features gracefully
- **Auth**: Bearer token → SHA256 hash → DB lookup → vault-scoped access
- **GraphQL**: schema at `internal/graph/schema.graphqls`, config at `gqlgen.yml`
- **Wiki-link resolution**: exact path match first, then title match (shortest path wins)
- **CLI uses GraphQL API**: `cmd/knowhow/` never connects directly to DB
- **Bootstrap connects directly to DB**: `cmd/bootstrap/` is a one-time setup script

### Running

```bash
# 1. Start SurrealDB
just db-up

# 2. Bootstrap (creates user, vault, API token)
go run -buildvcs=false ./cmd/bootstrap --name "Admin"
# Prints token to stdout, vault ID to stderr

# 3. Start server
just build-server
./bin/knowhow-server

# 4. Scrape documents
KNOWHOW_TOKEN=kh_... ./bin/knowhow scrape ./docs --vault <vault-id>
```

### Tests

```bash
just test  # All tests
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
