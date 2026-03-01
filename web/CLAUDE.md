# Webapp Template

## Tech Stack

- **Framework**: Next.js 16 (App Router, Turbopack)
- **Language**: TypeScript 5.9
- **Auth**: Auth.js v5 (OIDC) — in maintenance mode, Better Auth is the successor
- **Database**: PostgreSQL 17 + Drizzle ORM
- **Styling**: Tailwind CSS v4 (CSS-first config via `@theme` in `globals.css`)
- **Components**: Headless UI + Heroicons
- **Testing**: Vitest 4 (unit/storybook) + Playwright (E2E)
- **i18n**: next-intl (DE/EN)
- **Package Manager**: Bun

## First-Run Setup

Run all commands from the **monorepo root** (`../`):

```bash
just db-all              # start SurrealDB + PostgreSQL
just web-install         # bun install
just web-db-migrate      # run Drizzle migrations
just web-db-seed         # seed example data
just dev-all             # start everything (Go server + web dev)
```

## Dev Commands

All `just web-*` commands run from root and inherit env vars from `.env`:

```bash
just web-dev             # start dev server (port 3000)
just web-build           # production build
just web-test            # run Storybook tests
just web-test-e2e        # run Playwright E2E tests
just web-lint            # lint + typecheck
just web-db-migrate      # run Drizzle migrations
just web-db-seed         # seed example data
```

For commands not in justfile (storybook, format, db:generate), run from `web/`:

```bash
cd web && bun storybook        # start Storybook (port 6006)
cd web && bun run format       # format code with Prettier
cd web && bun run db:generate  # generate Drizzle migration
```

## Architecture

### Route Groups
- `app/(auth)/` — public auth pages (login)
- `app/(main)/` — authenticated pages (dashboard, settings)
- `app/api/` — API routes (auth handler, health check)

### Key Files
- `proxy.ts` — route protection (cookie check, redirects to /login). Next.js 16 convention replacing `middleware.ts`
- `app/lib/env.ts` — env var validation with lazy getters. Skips validation during `next build` via `NEXT_PHASE`
- `app/lib/auth.ts` — Auth.js config, session helpers, OIDC provider setup
- `app/lib/types.ts` — shared const arrays + derived types (`LOCALES`, `THEMES`)
- `app/lib/routes.ts` — centralized route constants
- `app/lib/action-result.ts` — `ActionResult` discriminated union for server actions
- `app/lib/action-utils.ts` — `parseFormData()` Zod validation helper

### Component Layers

Import direction: Pages → Domain → Composites → Primitives (never upward).

1. **Primitives** (`components/ui/`) — Headless UI wrappers (Button, Input, Dialog, etc.)
2. **Composites** (`components/`) — composed patterns (Card, FormField, AppShell, etc.)
3. **Domain** (`components/domain/`) — app-specific components (PostCard, CreatePostForm)

Pages import from Domain and Composites. Direct imports from `components/ui/` are acceptable for widely-used primitives (Button, Skeleton) when no composite wrapper exists.

### Database
- Schema: `app/lib/db/schema.ts` (Drizzle)
- Queries: `app/lib/db/*.ts`
- Migrations: `db/migrations/`
- Connection: `app/lib/db.ts` — singleton Pool cached on `globalThis` for HMR

### Server Actions
- Located in `app/lib/actions/`
- Return `ActionResult` type (`{ success: true } | { success: false; error: string }`)
- Use `ActionResultWith<T>` when success carries data
- Use `parseFormData()` for Zod validation
- Actions accept `(prevState, formData)` for `useActionState` compatibility
- Never export types from `"use server"` files — Turbopack errors on erased type exports

## Code Conventions

- Always use `bun` instead of `npm`/`node`
- Server-only code imports `"server-only"`
- Components use `cn()` from `@/lib/utils` for class merging
- Color tokens: `primary-*` (blue/indigo), `accent-*` (amber), `red-*` (error)
- i18n: all user-facing strings in `messages/{locale}.json`
- Theme: cookie-based with FOUC-preventing inline script
- Env vars: access via `env.DATABASE_URL` (from `app/lib/env.ts`), never raw `process.env` in app code
- React Compiler enabled (`reactCompiler: true`) — avoid manual `useMemo`/`useCallback`/`memo`
- Forms use `<form action={...}>` with `useActionState` for progressive enhancement
- `useFormStatus` for pending state — must be in a child component of the form
- Use `Suspense` boundaries around async server components for streaming
- Use `Promise.all` for parallel data fetching in server components

## Environment Variables

All env vars live in the **root `.env`** (not `web/.env.local`). The justfile loads them via `dotenv-load`. See root `.env.example` for the full list.

- `DATABASE_URL` — PostgreSQL connection string (required always)
- `NEXTAUTH_SECRET` — Auth.js session encryption (required in production)
- `NEXTAUTH_URL` — canonical app URL
- `AUTH_OIDC_*` — OIDC provider config (required in production)
- `APP_URL` — public app URL
- `KNOWHOW_SERVER_<NAME>_URL` — Go backend GraphQL endpoint per server
- `KNOWHOW_SERVER_<NAME>_TOKEN` — API token per server (from bootstrap)

## Security

- CSP enforced (not Report-Only) — inline theme script allowed via SHA-256 hash
- HSTS with 2-year max-age, includeSubDomains, preload
- Open redirect protection on `returnTo` params — validates relative paths only
- Proxy redirects authenticated users away from `/login`
- `server-only` import in all server modules (auth.ts, db.ts, env.ts)

## Gotchas

- **Docker build + env vars**: `env.ts` skips validation when `NEXT_PHASE=phase-production-build` so `next build` succeeds without runtime env vars
- **Root `not-found.tsx`**: Must be `"use client"` with `useTranslations` — root not-found is statically rendered, has no request context for `getTranslations`
- **MDX in Storybook**: `<` in MDX prose is parsed as JSX. Escape angle brackets or use backtick code spans
- **Tailwind v4 cursor**: Preflight resets `cursor: default` on buttons. Global override in `globals.css` restores `cursor: pointer`
- **`useActionState` signature**: Server actions used with `useActionState` need `(prevState, formData)` — not just `(formData)`
