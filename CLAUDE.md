# Knowhow

An MCP (Model Context Protocol) server in Go that provides a persistent knowledge layer for AI agents, backed by SurrealDB. Includes a bubbletea v2 TUI for agent chat and a WebDAV server for document editing.

## Project Status

This project is in active development with no production users. **Don't worry about backwards compatibility** — breaking changes to APIs, DB schema, SSE events, etc. are fine. Prefer clean designs over compatibility shims.

## Tech Stack

- **Backend**: Go, REST API, SurrealDB
- **TUI**: Bubbletea v2, Bubbles v2, Glamour, Lipgloss v2
- **WebDAV**: `golang.org/x/net/webdav` for document editing with any editor
- **Protocol**: MCP (Model Context Protocol)

## Commands

Use `just` for all build and test commands:

```bash
# Build
just build           # Single binary (CLI + server)

# Run
just bootstrap       # Wipe DB + create user/vault/token (requires running DB)
just dev             # Start Go dev environment (air, requires running DB)
just run serve       # Build and run server

# Test
just test            # Run Go tests
```

**IMPORTANT**: Before committing any changes, always run `just test`.

**IMPORTANT**: Always use `just build` instead of raw `go build ./...`. The justfile includes `-buildvcs=false` which is required because this project is in a subdirectory of the git repo. Raw `go build` will fail with `error obtaining VCS status: exit status 128`.

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

**Error wrapping context**: The `fmt.Errorf` context string describes the **caller's** operation, not the called function's. The called function already provides its own error message — wrapping adds the caller's context to build a chain:
```go
// GOOD — caller describes its own operation
func (s *Service) Create(...) error {
    _, err := s.db.CreateDocument(ctx, input)
    return fmt.Errorf("create: %w", err)
    // produces: "create: create document: connection refused"
}

// BAD — duplicates what CreateDocument already says
return fmt.Errorf("create document: %w", err)
// produces: "create document: create document: connection refused"
```

**Error strings must start with a lowercase letter** (per Go convention / `staticcheck ST1005`):
- `fmt.Errorf("create vault: %w", err)` — correct
- `fmt.Errorf("Create vault: %w", err)` — wrong
- Proper nouns and acronyms that are normally capitalized (e.g. `KNOWHOW_BEDROCK_*`, `HTTP 500`) are fine

## Structured Logging & Instrumentation

All new code should use the project's structured logging patterns:

### Context-carried loggers (`logutil`)

Always use `logutil.FromCtx(ctx)` instead of bare `slog.Debug/Info/Warn/Error`. This ensures request-scoped fields (`request_id`, `user_id`, `vault_id`) propagate automatically:

```go
logger := logutil.FromCtx(ctx)
logger.Debug("operation starting", "key", value)
```

When a function has a fixed attribute (e.g. model name), bake it into the logger once:
```go
logger := logutil.FromCtx(ctx).With("model", m.modelName)
logger.Debug("starting", "input_len", len(input))  // model is automatic
```

### DB operations — `defer c.logOp(ctx, op, time.Now())`

Every public DB query method must include `defer c.logOp(ctx, "table.operation", time.Now())` as its first line. This logs timing at Debug level AND records metrics. Do NOT also call `startOp`/`recordTiming` — `logOp` handles both.

```go
func (c *Client) GetFoo(ctx context.Context, id string) (*Foo, error) {
    defer c.logOp(ctx, "foo.get", time.Now())
    // ... query ...
}
```

For parent methods that delegate to private helpers (e.g. `UpsertAsset` → `createAsset`/`updateAsset`), only instrument the parent to avoid duplicate log lines.

### Middleware logger enrichment

When middleware adds context (e.g. `user_id`, `conversation_id`), enrich the context logger:
```go
logger := logutil.FromCtx(ctx).With("user_id", userID)
ctx = logutil.WithLogger(ctx, logger)
```

### Environment variables

All env vars use the `KNOWHOW_` prefix: `KNOWHOW_LOG_LEVEL`, `KNOWHOW_LOG_FILE`, etc.

## REST API

The server exposes a REST API at `/api/` for CLI and TUI communication:

- `GET /api/vaults` — list accessible vaults
- `GET /api/conversations?vault={id}` — list conversations
- `GET /api/conversations/{id}` — get conversation with messages
- `POST /api/conversations` — create conversation
- `DELETE /api/conversations/{id}` — delete conversation
- `PATCH /api/conversations/{id}` — rename conversation
- `GET /api/ls?vault={id}&path={path}&recursive=true` — list files and folders
- `GET /api/documents?vault={id}&path={path}` — get document by vault+path
- `POST /api/documents` — create/update document
- `GET /api/config` — server configuration

Agent endpoints (SSE streaming):
- `POST /agent/chat` — send message, receive SSE stream
- `POST /agent/approval` — approve/reject tool calls

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
- `docs/docker.md` - Docker Compose, Colima, SurrealDB v3 image gotchas
- `docs/teleport-proxy.md` - Teleport AWS proxy architecture, eino-ext bugs, CA cert handling
- `docs/sse.md` - Server-Sent Events, event bus, live updates

### WebDAV

The WebDAV server at `/dav/{vaultName}/` allows editing documents with any WebDAV-compatible editor. Auth uses HTTP Basic Auth where the password is a knowhow API token. Each vault gets its own lock system.

## Architecture — Vault-Based Document System

### Project Structure

```
cmd/knowhow/            # Single binary: CLI (cp, config, ui, dev seed) + server (serve)
internal/
├── models/             # Data structs + helpers (RecordIDString, ContentHash)
├── db/                 # SurrealDB client, DDL, query functions, connection
├── document/           # Document lifecycle: parse → embed → link → chunk
├── vault/              # Vault CRUD + virtual folder derivation
├── search/             # Hybrid BM25 + vector search with RRF fusion
├── template/           # Template CRUD
├── auth/               # Token auth, ValidateToken, AuthContext
├── server/             # Application bootstrap (DB, embedder, LLM, services)
├── api/                # REST API handlers (conversations, vaults, documents, config)
├── apiclient/          # Lightweight REST client for CLI/TUI
├── tui/                # Bubbletea v2 terminal UI (chat, conversations)
├── parser/             # Markdown parsing, wiki-link extraction, chunking
├── llm/                # LLM/embedding provider abstraction
├── config/             # Configuration loading
├── metrics/            # Metrics collection
├── event/              # In-process event bus (SSE change notifications)
├── webdav/             # WebDAV filesystem backed by document service
└── integration/        # Full lifecycle integration tests
```

### Key Patterns

- **SurrealDB v3 strict mode**: `option<T>` fields require `surrealmodels.None` (not Go `nil`/`NULL`)
- **HNSW indexes reject NONE**: Even on `option<array<float>>` fields, the HNSW index can't index NONE values. Omit the field from CREATE instead of setting it to NONE — the async embedding worker fills it in later via UPDATE
- **Record ID normalization**: DB queries use `type::record("vault", $id)` which expects a bare ID (`"default"`), not a prefixed one (`"vault:default"`). The `bareID(table, id)` helper in `internal/db/helpers.go` strips the prefix so callers can pass either format
- **Embedder is optional**: `nil` embedder disables AI features gracefully
- **Auth**: Bearer token → SHA256 hash → DB lookup → vault-scoped access
- **REST API**: JSON endpoints at `/api/`, auth via `Authorization: Bearer` header
- **Wiki-link resolution**: exact path match first, then title match (shortest path wins)
- **CLI uses REST API**: `cp`/`config`/`ui` commands communicate via REST
- **`serve` connects directly to DB**: starts the HTTP server (REST API, WebDAV, MCP, SSH)
- **`dev` commands connect directly to DB**: `dev seed` bootstraps schema + user/vault/token

### Running

```bash
# 1. Start SurrealDB (Docker) or use launchd — configure SURREALDB_URL in .env
just db-up

# 2. Bootstrap (wipe + create user/vault/token from justfile defaults)
just bootstrap

# 3. Start server with live-reload
just dev

# 4. Copy documents (KNOWHOW_TOKEN is set by justfile)
just run cp ./docs / --vault default

# 5. Launch TUI
just run ui
```

## Bubbletea v2 TUI

The TUI at `internal/tui/` provides agent chat via `knowhow ui`. Use the `bubbletea` subagent for TUI implementation.

### Import Paths (v2)

```go
import (
    "charm.land/bubbles/v2/textinput"
    tea "charm.land/bubbletea/v2"
    lipgloss "charm.land/lipgloss/v2"
    "github.com/charmbracelet/glamour"
)
```

### Key v2 API Changes

- `View()` returns `tea.View`, use `tea.NewView(content)` wrapper
- `tea.KeyMsg` → `tea.KeyPressMsg`
- `Init()` returns `tea.Cmd` only (not `(Model, Cmd)`)
- Alt screen via `View.AltScreen = true` (not program option)
- Lipgloss moved to `charm.land/lipgloss/v2`

### TUI Package Structure

- `app.go` — Root model: inline chat with tea.Println scrollback
- `render.go` — Markdown rendering, stream parts, message formatting
- `client.go` — REST client wrapper with SSE stream reader
- `styles.go` — Lipgloss v2 styles
- `keys.go` — Key binding definitions
