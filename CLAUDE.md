# Knowhow

An MCP (Model Context Protocol) server in Go that provides a persistent knowledge layer for AI agents, backed by SurrealDB. Includes a Next.js web frontend for browsing and editing documents.

## Tech Stack

- **Backend**: Go, GraphQL (gqlgen), SurrealDB
- **Frontend**: Next.js 16, TypeScript, Tailwind CSS v4 (fully stateless, no database)
- **Protocol**: MCP (Model Context Protocol)
- **Package Manager (web)**: Bun

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
just dev-all         # Start everything (SurrealDB + Go server + Web dev)

# Test
just test            # Run Go tests
just web-test        # Run web unit + storybook tests
just web-test-e2e    # Run Playwright E2E tests
just web-lint        # Lint + typecheck web

# Web
just web-install     # Install web dependencies (bun)
just web-dev         # Start Next.js dev server (:3000)
just web-build       # Production build
```

For commands not in justfile (storybook, format), run from `web/`:

```bash
cd web && bun storybook        # start Storybook (port 6006)
cd web && bun run format       # format code with Prettier
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
- `docs/nextjs.md` - Next.js 16 best practices, App Router, caching, security
- `docs/react.md` - React 19 patterns, Server Components, hooks, TypeScript
- `docs/gotchas.md` - Framework gotchas (Next.js, Tailwind v4, Zod 4, i18n)
- `docs/component-architecture.md` - Component layers, import rules, composition patterns
- `docs/design-system.md` - Color palette, typography, spacing, component styles
- `docs/design-system-best-practices.md` - General UI/UX design principles
- `docs/testing-strategy.md` - Testing pyramid: Vitest, Storybook, Playwright
- `docs/a11y.md` - Accessibility guide, axe-core, common violations

## Web Frontend

The web frontend lives in `web/` — a Next.js 16 app (App Router, Turbopack, TypeScript 5.9, Tailwind CSS v4, Headless UI, next-intl DE/EN). Fully stateless — auth uses encrypted cookie sessions (AES-256-GCM).

Environment vars live in the root `.env` (loaded by justfile's `dotenv-load`). Do **not** run `bun run dev` directly in `web/` — use `just web-dev` so vars are inherited.

### Web Architecture

**Route Groups:**
- `app/(auth)/` — public auth pages (login)
- `app/(main)/` — authenticated pages (docs, settings)
- `app/api/` — API routes (GraphQL proxy, health check)

**Key Files:**
- `proxy.ts` — route protection (cookie check, redirects to /login). Next.js 16 convention replacing `middleware.ts`
- `app/lib/env.ts` — env var validation with lazy getters. Skips validation during `next build` via `NEXT_PHASE`
- `app/lib/session.ts` — encrypted cookie session (AES-256-GCM). Stores server URL + API token
- `app/lib/auth.ts` — theme/locale cookie helpers
- `app/lib/types.ts` — shared const arrays + derived types (`LOCALES`, `THEMES`)
- `app/lib/routes.ts` — centralized route constants
- `app/lib/action-result.ts` — `ActionResult` discriminated union for server actions
- `app/lib/action-utils.ts` — `parseFormData()` Zod validation helper

**Component Layers** (import direction: Pages → Domain → Composites → Primitives, never upward):
1. **Primitives** (`components/ui/`) — Headless UI wrappers (Button, Input, Dialog, etc.)
2. **Composites** (`components/`) — composed patterns (Card, FormField, AppShell, etc.)
3. **Domain** (`components/domain/`) — app-specific components (VaultSwitcher, DocSidebar)

**Auth Flow:**
- User enters server URL + API token on `/login`
- Credentials encrypted into httpOnly cookie (`kh_session`) using `SESSION_SECRET`
- All GraphQL requests read the cookie server-side, forward token to Go backend
- No database, no OIDC — the Go backend's token system handles authorization

**Server Actions:**
- Located in `app/lib/actions/`
- Return `ActionResult` type (`{ success: true } | { success: false; error: string }`)
- Use `ActionResultWith<T>` when success carries data
- Use `parseFormData()` for Zod validation
- Actions accept `(prevState, formData)` for `useActionState` compatibility
- Never export types from `"use server"` files — Turbopack errors on erased type exports

### Code Conventions (Web)

- Always use `bun` instead of `npm`/`node`
- Server-only code imports `"server-only"`
- Components use `cn()` from `@/lib/utils` for class merging
- Color tokens: `primary-*` (blue/indigo), `accent-*` (amber), `red-*` (error)
- i18n: all user-facing strings in `messages/{locale}.json`
- Theme: cookie-based with FOUC-preventing inline script
- Env vars: access via `env.SESSION_SECRET` (from `app/lib/env.ts`), never raw `process.env` in app code
- React Compiler enabled (`reactCompiler: true`) — avoid manual `useMemo`/`useCallback`/`memo`
- Forms use `<form action={...}>` with `useActionState` for progressive enhancement
- `useFormStatus` for pending state — must be in a child component of the form
- Use `Suspense` boundaries around async server components for streaming
- Use `Promise.all` for parallel data fetching in server components

### Environment Variables (Web)

All env vars live in the **root `.env`** (not `web/.env.local`). The justfile loads them via `dotenv-load`. See root `.env.example` for the full list.

- `SESSION_SECRET` — encrypts session cookies (required in production)
- `APP_URL` — public app URL

### Security (Web)

- CSP enforced (not Report-Only) — inline theme script allowed via SHA-256 hash
- HSTS with 2-year max-age, includeSubDomains, preload
- Open redirect protection on `returnTo` params — validates relative paths only
- Proxy redirects authenticated users away from `/login`
- `server-only` import in all server modules (session.ts, env.ts)
- API tokens encrypted at rest in httpOnly cookies (AES-256-GCM)

### Gotchas (Web)

- **Docker build + env vars**: `env.ts` skips validation when `NEXT_PHASE=phase-production-build` so `next build` succeeds without runtime env vars
- **Root `not-found.tsx`**: Must be `"use client"` with `useTranslations` — root not-found is statically rendered, has no request context for `getTranslations`
- **Tailwind v4 cursor**: Preflight resets `cursor: default` on buttons. Global override in `globals.css` restores `cursor: pointer`
- **`useActionState` signature**: Server actions used with `useActionState` need `(prevState, formData)` — not just `(formData)`
- **Catch-all route params are URL-encoded**: In `[...path]` routes, `params.path` segments stay percent-encoded (e.g., `%20` for spaces). Always decode before using as DB keys: `path.map(decodeURIComponent).join("/")`
- **Server URL vs GraphQL endpoint**: The session stores the base server URL (e.g., `http://localhost:8484`). The `gql` client appends `/query` automatically. Login accepts either format and strips `/query` if present

## Architecture — Vault-Based Document System

### Project Structure

```
cmd/knowhow-server/     # GraphQL server
cmd/knowhow/            # CLI client (scrape command, uses GraphQL API)
cmd/bootstrap/          # One-time script: creates user + vault + token
web/                    # Next.js frontend
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

# 2. Start server
just dev          # or: just dev-all (includes web)

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
