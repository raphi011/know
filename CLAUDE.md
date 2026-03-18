# Know

An MCP (Model Context Protocol) server in Go that provides a persistent knowledge layer for AI agents, backed by SurrealDB. Includes a bubbletea v2 TUI for agent chat, a WebDAV server for document editing, and an NFS server for fast file access.

## Project Status

This project is in active development with no production users. **Don't worry about backwards compatibility** — breaking changes to APIs, DB schema, SSE events, etc. are fine. Prefer clean designs over compatibility shims.

## Tech Stack

- **Backend**: Go, REST API, SurrealDB
- **TUI**: Bubbletea v2, Bubbles v2, Glamour, Lipgloss v2
- **WebDAV**: `golang.org/x/net/webdav` for document editing with any editor
- **Protocol**: MCP (Model Context Protocol)
- **Go version**: 1.26 — `new(value)` creates a pointer to a value (e.g. `new(42)` returns `*int` pointing to 42, `new(len(s))` returns `*int`). This replaces the old `func ptr[T any](v T) *T` helper pattern.
- **Indentation**: This project uses **tabs** for Go files (per `gofmt`). When editing files with the Edit tool, match the surrounding indentation exactly — the Edit tool's `old_string` must reproduce the file's literal whitespace (tabs, not spaces). Never use `sed` to insert lines, as macOS `sed` does not expand `\t` to tabs.

## Commands

Use `just` for all build and test commands:

```bash
# Build
just build           # Single binary (CLI + server)

# Run
just bootstrap       # Wipe DB + create user/vault/token (requires running DB)
just dev             # Build and start dev server (requires running DB)
just run serve       # Build and run server

# Test
just test            # Run Go tests
```

**IMPORTANT**: Before committing any changes, always run `just test`.

**IMPORTANT**: Always use `just build` instead of raw `go build ./...`. The justfile includes `-buildvcs=false` which is required because this project is in a subdirectory of the git repo. Raw `go build` will fail with `error obtaining VCS status: exit status 128`.

## SurrealDB Reference

For SurrealDB-specific syntax, v3.0 breaking changes, and query patterns:
- **Subagent**: Use the `surrealdb` subagent for complex query work (has built-in reference guide)

**IMPORTANT**: Every public `internal/db/` query method must have an integration test in the corresponding `queries_*_test.go` file. Tests run against a real SurrealDB instance via testcontainers — no mocking. When adding a new query method, add its test in the same PR.

**IMPORTANT**: Avoid N+1 queries, especially on hot paths (search, list endpoints, document processing). Prefer batch operations (`INSERT INTO table $rows`, `WHERE id IN $ids`) over looping single-record queries.

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
- Proper nouns and acronyms that are normally capitalized (e.g. `KNOW_BEDROCK_*`, `HTTP 500`) are fine

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

### Prometheus metrics (`internal/metrics`)

All metrics are defined in `internal/metrics/collector.go` with the `know_` prefix. When adding new features, keep metrics up to date:

- **New DB operations**: Automatically covered by `logOp` → `RecordTiming` (no extra work).
- **New HTTP endpoints**: Automatically covered by the metrics middleware → `RecordHTTPRequest`.
- **New LLM/embedding calls**: Call `metrics.RecordLLMUsage` or `metrics.RecordEmbedding` at the call site.
- **New pipeline job types**: Automatically covered — `PipelineWorker` calls `RecordPipelineJob` on completion/failure.
- **New subsystems** (e.g. a new external service, a new background worker): Add a new `*prometheus.HistogramVec` or `*prometheus.CounterVec` to the `Metrics` struct with a `Record*` method. All `Record*` methods must be nil-safe (`if m == nil { return }`).

When removing or renaming a metric, check for Grafana dashboard references or alerting rules before deleting.

### Environment variables

All env vars use the `KNOW_` prefix: `KNOW_LOG_LEVEL`, `KNOW_LOG_FILE`, etc.

## REST API

The server exposes a REST API at `/api/` for CLI and TUI communication, and agent endpoints at `/agent/`. Auth via `Authorization: Bearer` header.

All routes are registered in `internal/api/server.go` (REST) and `cmd/know/cmd_serve.go` (agent). Key resource groups:

- **Vaults**: list, info, settings (get/patch)
- **Conversations**: CRUD + rename
- **Documents**: get, upsert, delete, move, ls, bulk upload
- **Import/Export**: two-phase manifest+upload, tar.gz export, EPUB export
- **Assets**: upload, get, get meta, delete (binary blob storage)
- **Search**: hybrid BM25+vector search with filters
- **Tasks**: list (filterable), toggle status
- **Folders**: list, update settings (no_embed flag)
- **Labels/Versions**: list endpoints
- **External Links**: stats by hostname, paginated list
- **Jobs**: pipeline job stats, active jobs, recent failures
- **Remotes**: CRUD for multi-server federation
- **Web Clipping**: fetch web page as markdown and save to vault
- **Config**: server configuration
- **Agent**: chat (start), events (SSE stream), cancel, approval

## Documentation

**IMPORTANT**: When adding or modifying features, always update the relevant `docs/feature-*.md` file with example prompts showcasing what the feature can do. This helps users understand how to use each tool effectively.

### Feature Docs (`docs/feature-*.md`)

Each major feature has its own documentation file:
- `docs/feature-agent.md` - Agent chat, TUI, tool approval, SSE streaming
- `docs/feature-apple-app.md` - iOS/macOS app, quick picker, vault browsing
- `docs/feature-audio-transcription.md` - STT pipeline, Whisper integration, template summarization
- `docs/feature-ingestion.md` - Document pipeline, import command, frontmatter, wiki-links, versioning
- `docs/feature-mcp.md` - MCP server, tools, multi-instance setup, Claude Code/Cursor config
- `docs/feature-memory.md` - Memory system, decay scoring, consolidation, archiving
- `docs/feature-nfs.md` - NFS file access, mount instructions for macOS/Linux/Windows
- `docs/feature-remotes.md` - Multi-server federation, namespace routing
- `docs/feature-search.md` - Hybrid BM25+vector search, RRF fusion, LLM synthesis
- `docs/feature-ssh-sftp.md` - SSH/SFTP file access, CLI and GUI client setup
- `docs/feature-tasks.md` - Task extraction from checkboxes, filtering, toggling, API
- `docs/feature-templates.md` - Vault templates for document generation and summarization
- `docs/feature-vaults.md` - Vaults, folders, access control, share links
- `docs/feature-web-clipping.md` - Web page fetching via Jina Reader, CLI/MCP/API, vault settings
- `docs/feature-webdav.md` - WebDAV editing, auth, editor support

### Project-Specific Docs (`docs/`)

- `docs/teleport-proxy.md` - Teleport AWS proxy architecture, eino-ext bugs, CA cert handling
- `docs/testing-tui.md` - TUI testing strategy: direct component testing without teatest, state machine testing patterns

### Reusable Knowledge

Best practices, framework gotchas, and library references have been moved to the personal knowledge base (`~/Git/second-brain`). Search there for SurrealDB, embeddings, RAG, LLM, HTMX, Templ, Tailwind, WebDAV, Bubbletea, langchaingo, Docker, SSE, MCP, Next.js, React, accessibility, and testing patterns.

## Architecture — Vault-Based Document System

### Project Structure

```
cmd/know/            # Single binary: CLI (import, info, agent, job, db seed, db wipe) + server (serve)
internal/
├── agent/              # Agent orchestration (tool execution, conversation management)
├── api/                # REST API handlers (conversations, vaults, documents, config)
├── apiclient/          # Lightweight REST client for CLI/TUI
├── apify/              # Apify web scraping integration
├── auth/               # Token auth, ValidateToken, AuthContext
├── blob/               # Blob storage for binary files (images, PDFs)
├── config/             # Configuration loading
├── db/                 # SurrealDB client, DDL, query functions, connection
├── diff/               # Document diff computation
├── epub/               # EPUB export for documents and folders
├── event/              # In-process event bus (SSE change notifications)
├── file/               # File lifecycle: parse → embed → link → chunk
├── integration/        # Full lifecycle integration tests
├── jina/               # Jina Reader client for web page fetching
├── llm/                # LLM/embedding provider abstraction
├── logutil/            # Structured logging helpers (context-carried loggers)
├── mcptools/           # MCP tool definitions and handlers
├── memory/             # Memory system (decay scoring, consolidation, archiving)
├── metrics/            # Metrics collection
├── models/             # Data structs + helpers (RecordIDString, ContentHash)
├── nfs/                # NFSv3 server for fast file access (go-nfs + billy)
├── parser/             # Markdown parsing, wiki-link extraction, chunking
├── pathutil/           # Path normalization and manipulation helpers
├── pipeline/           # Pipeline job definitions and handlers (PDF, STT, embed)
├── remote/             # Multi-server federation and namespace routing
├── search/             # Hybrid BM25 + vector search with RRF fusion
├── server/             # Application bootstrap (DB, embedder, LLM, services)
├── sshd/               # SSH/SFTP server for file access
├── stt/                # Speech-to-text transcription
├── tools/              # Agent tool implementations
├── tui/                # Bubbletea v2 terminal UI (chat, conversations)
├── vault/              # Vault CRUD + virtual folder derivation
├── webclip/            # Web clipping service (fetch + save web pages)
├── webdav/             # WebDAV filesystem backed by document service
└── worker/             # Background worker (job processing, scheduling)
```

### Key Patterns

- **SurrealDB v3 strict mode**: `option<T>` fields require `surrealmodels.None` (not Go `nil`/`NULL`)
- **HNSW indexes reject NONE**: Even on `option<array<float>>` fields, the HNSW index can't index NONE values. Omit the field from CREATE instead of setting it to NONE — the async embedding worker fills it in later via UPDATE
- **Record ID normalization**: DB queries use `type::record("vault", $id)` which expects a bare ID (`"default"`), not a prefixed one (`"vault:default"`). The `bareID(table, id)` helper in `internal/db/helpers.go` strips the prefix so callers can pass either format
- **Embedder is optional**: `nil` embedder disables AI features gracefully
- **Auth**: Bearer token → SHA256 hash → DB lookup → vault-scoped access
- **REST API**: JSON endpoints at `/api/`, auth via `Authorization: Bearer` header
- **Wiki-link resolution**: exact path match first, then title match (shortest path wins)
- **CLI uses REST API**: `import`/`info`/`agent` commands communicate via REST
- **`serve` connects directly to DB**: starts the HTTP server (REST API, WebDAV, MCP, SSH)
- **`db` commands connect directly to DB**: `db seed` bootstraps schema + user/vault/token, `db wipe` clears all data

### Running

```bash
# 1. Start SurrealDB (Docker) or use launchd — configure SURREALDB_URL in .env
just db-up

# 2. Bootstrap (wipe + create user/vault/token from justfile defaults)
just bootstrap

# 3. Start server with live-reload
just dev

# 4. Import documents (KNOW_TOKEN is set by justfile)
just run import ./docs / --vault default -y

# 5. Launch TUI
just run agent
```

## Bubbletea v2 TUI

The TUI at `internal/tui/` provides agent chat via `know agent`. Use the `bubbletea` subagent for TUI implementation.

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
- `attachment.go` — File attachment handling for agent messages
- `client.go` — REST client wrapper with SSE stream reader
- `filelist.go` — File list browsing component
- `keys.go` — Key binding definitions
- `render.go` — Markdown rendering, stream parts, message formatting
- `styles.go` — Lipgloss v2 styles
- `tasks.go` — Task list display and interaction
