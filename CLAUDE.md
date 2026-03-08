# Knowhow

An MCP (Model Context Protocol) server in Go that provides a persistent knowledge layer for AI agents, backed by SurrealDB. Includes a Go Templ + HTMX web frontend and a WebDAV server for document editing.

## Project Status

This project is in active development with no production users. **Don't worry about backwards compatibility** — breaking changes to APIs, DB schema, SSE events, etc. are fine. Prefer clean designs over compatibility shims.

## Tech Stack

- **Backend**: Go, GraphQL (gqlgen), SurrealDB
- **Frontend**: Go Templ + HTMX, Tailwind CSS (server-rendered, no JS framework)
- **WebDAV**: `golang.org/x/net/webdav` for document editing with any editor
- **Protocol**: MCP (Model Context Protocol)

## Commands

Use `just` for all build and test commands:

```bash
# Build
just build           # CLI binary
just build-server    # GraphQL server
just build-bootstrap # Bootstrap script
just build-all       # All binaries
just generate        # Regenerate gqlgen code

# Run
just bootstrap       # Wipe DB + create user/vault/token from env var defaults
just dev             # Start Go dev environment (air)
just dev-all         # Start everything (SurrealDB + Go server)

# Test
just test            # Run Go tests
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

**Error strings must start with a lowercase letter** (per Go convention / `staticcheck ST1005`):
- `fmt.Errorf("create vault: %w", err)` — correct
- `fmt.Errorf("Create vault: %w", err)` — wrong
- Proper nouns and acronyms that are normally capitalized (e.g. `KNOWHOW_BEDROCK_*`, `HTTP 500`) are fine

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
- `docs/docker.md` - Docker Compose, Colima, SurrealDB v3 image gotchas
- `docs/teleport-proxy.md` - Teleport AWS proxy architecture, eino-ext bugs, CA cert handling
- `docs/sse.md` - Server-Sent Events, event bus, live updates
- `docs/templ.md` - Go Templ patterns, component composition, rendering
- `docs/htmx.md` - HTMX attributes, partials, SSE extension
- `docs/tailwind-css.md` - Tailwind v4, semantic tokens, dark mode, build

## Web Frontend (Templ + HTMX)

The web frontend is server-rendered Go using [Templ](https://templ.guide/) templates and [HTMX](https://htmx.org/) for interactivity. No JS build step, no Node.js.

### Key Files

- `internal/web/handler.go` — HTTP handlers (login, doc view, settings, HTMX partials)
- `internal/web/sidebar.go` — sidebar tree builder
- `internal/web/session.go` — in-memory session store (`sync.Map`, mutex-protected fields)
- `internal/web/middleware.go` — session middleware, `SessionFromContext`
- `internal/web/csrf.go` — CSRF protection via Origin/Referer checking
- `internal/web/render.go` — goldmark markdown renderer (GFM, no unsafe HTML)
- `internal/web/i18n.go` — embedded JSON i18n (EN/DE), `T(locale)` function
- `internal/web/templates/` — Templ components (pages, partials, components)
- `internal/web/static/` — embedded static assets (CSS, JS)
- `internal/web/messages/` — i18n JSON files

### Architecture

- **Templates**: Templ generates Go code from `.templ` files. Run `templ generate` after editing templates
- **HTMX partials**: `GET /hx/*` endpoints return HTML fragments for in-page swaps
- **SSE**: `GET /hx/doc/events` streams live document updates via Server-Sent Events (event bus)
- **Auth**: Token validated via `auth.ValidateToken()` → in-memory session → `kh_sid` cookie
- **CSRF**: Origin/Referer header check on all POST endpoints
- **Buffered rendering**: HTMX partial handlers use `renderBuffered()` to prevent partial HTML on errors
- **i18n**: `messages/en.json` and `messages/de.json`, accessed via `T(locale)("key")`

### WebDAV

The WebDAV server at `/dav/{vaultName}/` allows editing documents with any WebDAV-compatible editor. Auth uses HTTP Basic Auth where the password is a knowhow API token. Each vault gets its own lock system.

## Architecture — Vault-Based Document System

### Project Structure

```
cmd/knowhow-server/     # GraphQL server + web UI + WebDAV
cmd/knowhow/            # CLI client (scrape command, uses GraphQL API)
cmd/bootstrap/          # One-time script: creates user + vault + token
internal/
├── models/             # Data structs + helpers (RecordIDString, ContentHash)
├── db/                 # SurrealDB client, DDL, query functions, connection
├── document/           # Document lifecycle: parse → embed → link → chunk
├── vault/              # Vault CRUD + virtual folder derivation
├── search/             # Hybrid BM25 + vector search with RRF fusion
├── template/           # Template CRUD
├── auth/               # Token auth, ValidateToken, AuthContext
├── graph/              # GraphQL schema, resolvers, gqlgen config
├── parser/             # Markdown parsing, wiki-link extraction, chunking
├── llm/                # LLM/embedding provider abstraction
├── config/             # Configuration loading
├── metrics/            # Metrics collection
├── event/              # In-process event bus (SSE change notifications)
├── web/                # Templ + HTMX web frontend
│   ├── templates/      # Templ components (pages, partials, components)
│   ├── static/         # Embedded CSS/JS assets
│   └── messages/       # i18n JSON files (en, de)
├── webdav/             # WebDAV filesystem backed by document service
└── integration/        # Full lifecycle integration tests
```

### Key Patterns

- **SurrealDB v3 strict mode**: `option<T>` fields require `surrealmodels.None` (not Go `nil`/`NULL`)
- **HNSW indexes reject NONE**: Even on `option<array<float>>` fields, the HNSW index can't index NONE values. Omit the field from CREATE instead of setting it to NONE — the async embedding worker fills it in later via UPDATE
- **Record ID normalization**: DB queries use `type::record("vault", $id)` which expects a bare ID (`"default"`), not a prefixed one (`"vault:default"`). The `bareID(table, id)` helper in `internal/db/helpers.go` strips the prefix so callers can pass either format
- **Embedder is optional**: `nil` embedder disables AI features gracefully
- **Auth**: Bearer token → SHA256 hash → DB lookup → vault-scoped access
- **GraphQL**: schema at `internal/graph/schema.graphqls`, config at `gqlgen.yml`
- **Wiki-link resolution**: exact path match first, then title match (shortest path wins)
- **CLI uses GraphQL API**: `cmd/knowhow/` never connects directly to DB
- **Bootstrap connects directly to DB**: `cmd/bootstrap/` is a one-time setup script

### Running

```bash
# 1. Bootstrap (starts SurrealDB, wipes, creates user/vault/token from justfile defaults)
just bootstrap

# 2. Start server (includes web UI at /login)
just dev

# 3. Scrape documents (KNOWHOW_TOKEN is set by justfile)
just run scrape ./docs --vault vault:default
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
