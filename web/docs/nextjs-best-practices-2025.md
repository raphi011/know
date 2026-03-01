# Next.js Best Practices 2025/2026

> Reference document for auditing the pyramid codebase against current Next.js conventions.
> Focuses on framework-specific patterns. For React-level practices (hooks, state, TypeScript), see `react-best-practices-2025.md`.
> Each section includes an **Audit checklist** for comparison.

---

## 1. App Router Conventions

The App Router uses a **file-system based** routing model. Every folder in `app/` maps to a URL segment. Special files control behavior at each route segment.

### Special Files

| File | Purpose |
|------|---------|
| `page.tsx` | Route UI — makes the segment publicly accessible |
| `layout.tsx` | Shared UI wrapping child routes — preserves state across navigations |
| `loading.tsx` | Suspense fallback for the segment — shows while page data loads |
| `error.tsx` | Error boundary for the segment — catches rendering errors |
| `not-found.tsx` | UI for `notFound()` calls — 404 handling |
| `global-error.tsx` | Root-level error boundary — catches errors in root layout |
| `template.tsx` | Like layout but re-mounts on navigation — useful for enter/exit animations |
| `default.tsx` | Fallback for parallel route slots when no matching segment |
| `proxy.ts` | Request interception (Next.js 16, replaces `middleware.ts`) |

### Route Groups

- `(groupName)/` — organizes routes without affecting the URL
- Common use: `(auth)/login`, `(main)/dashboard` — separate layouts for auth vs app shell
- Enable **multiple root layouts** at the same level

### Key Rules

- **Layouts don't re-render** on navigation between child routes — don't fetch data that changes per page in a layout
- **`page.tsx` is required** to make a route accessible — a folder without `page.tsx` is just organizational
- **`searchParams` is a Promise** in Next.js 15+ — must `await` in Server Components
- **`params` is also a Promise** in Next.js 15+ — must `await` in dynamic routes

### Audit Checklist

- [ ] Route groups `(groupName)` used to separate layout contexts (auth vs main)
- [ ] `loading.tsx` exists for pages with async data
- [ ] `error.tsx` exists for pages where errors are recoverable
- [ ] `not-found.tsx` exists for dynamic routes (user profiles, match detail, etc.)
- [ ] No data fetching in layouts that changes per child page
- [ ] `searchParams` and `params` are properly awaited (not used synchronously)

---

## 2. Rendering Strategies

Next.js decides rendering strategy per route based on how you access data.

### Static Rendering (Default)

- Routes rendered at **build time** — result cached and served from CDN
- Fastest option for pages that don't depend on request-specific data
- Triggered when: no dynamic functions (`cookies()`, `headers()`, `searchParams`) used

### Dynamic Rendering

- Routes rendered at **request time** — every request gets a fresh render
- Triggered when: any dynamic function is called, or `export const dynamic = 'force-dynamic'`
- Use for: personalized content, real-time data, auth-gated pages

### Streaming

- Server sends UI in **chunks** — user sees the shell immediately
- `<Suspense>` boundaries mark streaming points
- `loading.tsx` is syntactic sugar for a Suspense boundary around the page

### Cache Components (Replaces PPR)

- Combines static shell with dynamic holes — the layout renders statically, dynamic parts stream in
- Enabled globally with `cacheComponents: true` in `next.config.ts`, then use `"use cache"` directive per component/page
- Replaces the old `experimental.ppr` and `experimental.dynamicIO` flags
- Best of both worlds: instant static shell + dynamic personalized content

### Best Practices

- **Default to static** — only opt into dynamic when you need request-specific data
- **Avoid accidental dynamic rendering**: calling `cookies()` or `headers()` anywhere in the tree makes the entire route dynamic
- **Use `<Suspense>` strategically** to keep parts of the page static while streaming dynamic content
- **Don't use `export const dynamic = 'force-dynamic'` globally** — apply per-route only when needed

### Audit Checklist

- [ ] Pages without user-specific data are statically rendered
- [ ] No accidental `cookies()`/`headers()` calls making public pages dynamic
- [ ] Streaming used for pages with slow data sources
- [ ] `loading.tsx` files provide instant visual feedback during navigation

---

## 3. Caching & Revalidation

Next.js 16 introduces **explicit opt-in caching** via `"use cache"`. Previous implicit caching behavior is removed — all code is dynamic by default unless you cache it.

### `"use cache"` Directive (Next.js 16)

```tsx
"use cache"

export default async function Page() {
  const data = await db.query(...)
  return <DataTable data={data} />
}
```

- Place at top of file (caches entire page/component) or wrap specific functions
- Works with **any async operation** — not just `fetch` (database queries, file reads, computations)
- Replaces the old `fetch(..., { cache: 'force-cache' })` pattern

### `cacheLife` — Control Cache Duration

```tsx
import { cacheLife } from 'next/cache'

async function getStandings() {
  "use cache"
  cacheLife('hours')  // Built-in: 'seconds' | 'minutes' | 'hours' | 'days' | 'weeks' | 'max'
  return db.query(...)
}
```

- Only **one** `cacheLife` call per function invocation
- Custom profiles can be defined in `next.config.js`
- Explicit `cacheLife` always takes precedence over inferred lifetimes

### `cacheTag` — On-Demand Revalidation

```tsx
import { cacheTag } from 'next/cache'

async function getStandings(seasonId: string) {
  "use cache"
  cacheTag(`standings-${seasonId}`)
  return db.query(...)
}

// In a Server Action after a match result:
import { revalidateTag } from 'next/cache'
revalidateTag(`standings-${seasonId}`)
```

- Tag any cached computation for targeted revalidation
- Works with database queries, not just `fetch`

### `updateTag` — Read-Your-Writes (Next.js 16.1)

```tsx
import { updateTag } from 'next/cache'

// In a Server Action — the updated data is immediately visible to the current user
updateTag(`post-${postId}`, updatedPostData)
```

- Ensures the user who mutated data sees their changes immediately
- Other users see the update after cache revalidation

### `refresh` — Refresh Uncached Data

```tsx
import { refresh } from 'next/cache'

// Force-refresh data that isn't explicitly cached
refresh()
```

- Useful for refreshing dynamic data without a full page reload

### ISR (Still Supported)

- Time-based: `export const revalidate = 60` (seconds)
- On-demand: `revalidatePath('/rankings')` or `revalidateTag('standings')`
- Best used as **safety net** alongside on-demand revalidation

### Hybrid Strategy (Recommended)

- **Time-based revalidation** as a safety net (ensures staleness is bounded)
- **On-demand revalidation** via `revalidateTag`/`revalidatePath` after mutations (ensures immediate freshness)
- **Read-your-writes** via `updateTag` for instant feedback to the mutating user
- If webhook or mutation fails to trigger revalidation, time-based catches it

### Audit Checklist

- [ ] Evaluate `"use cache"` adoption for expensive/repeated queries
- [ ] On-demand revalidation (`revalidatePath`/`revalidateTag`) after every Server Action mutation
- [ ] Consider `updateTag` for read-your-writes in Server Actions
- [ ] Static pages use ISR or `"use cache"` with `cacheLife` instead of full dynamic rendering
- [ ] No stale data after mutations (revalidation fires in the same Server Action)
- [ ] Cache tags are granular enough for targeted invalidation

---

## 4. Advanced Routing

### Parallel Routes

- Define with `@slotName/` folders — render multiple pages simultaneously in one layout
- Use case: dashboard with independent panels, modals alongside main content
- Each slot gets its own `loading.tsx`, `error.tsx`, and navigation state
- **`default.tsx` is mandatory** for every slot — builds **fail** without them in Next.js 16

```
app/
  @sidebar/
    page.tsx
    default.tsx
  @main/
    page.tsx
  layout.tsx      <- receives { sidebar, main } as props
```

### Intercepting Routes

- Load a route within the current layout without full navigation
- Convention: `(.)segment` (same level), `(..)segment` (one level up), `(...)segment` (from root)
- Use case: modal that shows a preview (intercepted) but supports direct URL access (full page)
- **Must use `<Link>` component** — `<a>` tags bypass interception

### Combining for Modals

```
app/
  @modal/
    (.)photo/[id]/
      page.tsx     <- modal version (intercepted)
    default.tsx    <- renders nothing by default
  photo/[id]/
    page.tsx       <- full page version (direct URL)
  layout.tsx       <- renders {children} + {modal}
```

- Clicking a link shows the modal overlay
- Direct URL navigation shows the full page
- Deep-linking works — sharing the URL shows the full page

### Dynamic Routes

- `[slug]/` — single dynamic segment
- `[...slug]/` — catch-all (matches any depth)
- `[[...slug]]/` — optional catch-all (also matches the root)
- **`generateStaticParams`** for static generation of dynamic routes

### Audit Checklist

- [ ] Parallel routes used where independent UI panels exist (dashboard, modals)
- [ ] Intercepting routes used for preview modals that also work as standalone pages
- [ ] All parallel route slots have `default.tsx` fallbacks
- [ ] Dynamic routes use `generateStaticParams` where content is known at build time
- [ ] `<Link>` used instead of `<a>` for internal navigation (required for interception)

---

## 5. Proxy (Route Protection)

Next.js 16 renames `middleware.ts` to `proxy.ts` with the export renamed from `middleware` to `proxy`.

### Purpose

- Runs **before every route** (including prefetches)
- Modify request/response headers, redirect, rewrite, or respond directly
- Runs at the edge — must be fast and lightweight

### Best Practices

- **Optimistic checks only**: Read session cookie, check existence — never hit the database
- **Never use as sole auth layer**: Proxy checks are a first line of defense, not a replacement for Server Component/Action authorization
- **Keep the matcher tight**: Use `config.matcher` to exclude static assets, API routes you don't need to protect
- **Avoid heavy computation**: This runs on every request — keep it minimal
- **Don't call external APIs**: Adds latency to every request

### Multi-Layer Auth Pattern

```
proxy.ts          -> Optimistic: redirect if no session cookie
Server Component  -> Verify session is valid (DB check)
Server Action     -> Re-verify authorization for the specific mutation
```

### Common Patterns

```ts
// proxy.ts
import { NextRequest, NextResponse } from 'next/server'

export function proxy(request: NextRequest) {
  const session = request.cookies.get('session_token')
  const isAuthPage = request.nextUrl.pathname.startsWith('/login')

  if (!session && !isAuthPage) {
    return NextResponse.redirect(new URL('/login', request.url))
  }
  if (session && isAuthPage) {
    return NextResponse.redirect(new URL('/rankings', request.url))
  }
  return NextResponse.next()
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
}
```

### Audit Checklist

- [ ] `proxy.ts` does only optimistic cookie checks (no DB calls)
- [ ] Authorization re-verified in Server Components and Server Actions
- [ ] `config.matcher` excludes static assets and public routes
- [ ] Redirect URLs validated (no open redirect via `returnTo` params)
- [ ] Protected routes redirect to login, auth pages redirect to app when session exists
- [ ] `returnTo` parameter validated (starts with `/`, no `//`, no `:`) — see `proxy.ts`

---

## 6. Server Actions

Server Actions are async functions that execute on the server, callable from both Server and Client Components.

### When to Use

| Use Case | Approach |
|----------|----------|
| Form mutations (create, update, delete) | Server Actions via `<form action={...}>` |
| Data fetching (reads) | Server Components or Route Handlers |
| External API consumption | Route Handlers |
| Webhooks | Route Handlers |

### Best Practices

- **Validate all input** with `zod` or similar — client validation is UX, server validation is security
- **Check authorization** in every Server Action — don't rely on proxy alone
- **Return structured errors**, not thrown exceptions — use `useActionState` to consume them
- **Revalidate cache** in the same action after successful mutation
- **Keep actions thin**: validate -> authorize -> mutate -> revalidate -> return result
- **Use `redirect()` outside try/catch** — it throws internally and gets swallowed in catch blocks
- **Colocate with the feature** or centralize in `lib/actions/` — avoid scattering across components

### Progressive Enhancement

- `<form action={serverAction}>` works **without JavaScript**
- The form submits as a standard POST, server processes it, page refreshes
- When JS is available, React intercepts for SPA-like experience
- **Never break this**: don't make form submission depend on client-only state

### CSRF Protection

- Server Actions only accept POST requests
- Origin header is validated against Host header automatically
- SameSite cookies (default in modern browsers) prevent cross-site submission
- For extra protection, consider `@edge-csrf/nextjs`

### Audit Checklist

- [ ] Every Server Action validates input (zod schema)
- [ ] Every Server Action checks authorization independently
- [ ] `redirect()` is never called inside a try/catch
- [ ] Cache revalidation happens in the same action after mutation
- [ ] Server Actions used for writes only (not data fetching)
- [ ] Forms work without JavaScript (progressive enhancement)

---

## 7. Route Handlers (API Routes)

Route Handlers in `app/api/` handle HTTP requests explicitly — use when you need RESTful endpoints.

### When to Use

- **External API consumers**: Mobile apps, third-party integrations, webhooks
- **Explicit HTTP control**: Custom status codes, headers, streaming responses
- **Webhooks**: Signature verification + POST handling
- **Don't use for**: Internal mutations from React components (use Server Actions instead)

### Best Practices

- **Validate input early**: Use zod to validate request body/params before processing
- **Return meaningful errors**: Include context-specific error info, not generic 500s
- **Verify webhook signatures**: Always — skipping lets attackers forge events
- **Use proper HTTP methods**: GET for reads, POST for creates, PATCH/PUT for updates, DELETE for deletes
- **Include request ID in error responses**: Correlate client reports with server logs

### Caching Behavior (Next.js 15+)

- **GET handlers default to dynamic** (uncached) — set `export const dynamic = 'force-static'` for static
- Previous versions cached GET by default — this changed in Next.js 15

### Response Patterns

```ts
// Structured error response
return NextResponse.json(
  { error: 'Validation failed', details: result.error.flatten() },
  { status: 400 }
)

// Streaming response
return new Response(stream, {
  headers: { 'Content-Type': 'text/event-stream' },
})
```

### Audit Checklist

- [ ] Route Handlers used only for external APIs, webhooks, and explicit HTTP needs
- [ ] Internal mutations use Server Actions, not Route Handlers
- [ ] Input validated with zod in every handler
- [ ] Meaningful error responses with appropriate HTTP status codes
- [ ] Webhook handlers verify signatures
- [ ] Logging includes request identifiers for debugging

---

## 8. Metadata & SEO

### Static Metadata

```tsx
// layout.tsx or page.tsx
export const metadata: Metadata = {
  title: 'Pyramid Rankings',
  description: '...',
}
```

### Dynamic Metadata

```tsx
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const club = await getClub(slug)
  return {
    title: club.name,
    description: `Rankings for ${club.name}`,
    openGraph: { images: [club.imageUrl] },
  }
}
```

### Best Practices

- **Set `metadataBase`** in root `layout.tsx` — applies to all URL-based metadata fields
- **Every page should have a unique `title` and `description`**
- **Use `generateMetadata`** for dynamic routes (club pages, player profiles)
- **OpenGraph images**: Use `opengraph-image.tsx` file convention for dynamic OG images, or static files
- **Canonical URLs**: Prevent duplicate content issues
- **`robots.txt`** and **`sitemap.xml`**: Use file conventions in `app/` or generate dynamically
- **Structured data** (JSON-LD): Add in page/layout for rich search results

### Template Pattern

```tsx
// Root layout
export const metadata: Metadata = {
  title: {
    template: '%s | Pyramid',
    default: 'Pyramid Rankings',
  },
}

// Child page
export const metadata: Metadata = {
  title: 'TC Modling',  // Renders as "TC Modling | Pyramid"
}
```

### Audit Checklist

- [ ] `metadataBase` set in root layout
- [ ] Title template (`%s | AppName`) defined in root layout
- [ ] Dynamic routes use `generateMetadata` for per-page titles/descriptions
- [ ] OpenGraph images configured for shared links
- [ ] `robots.txt` and `sitemap.xml` present
- [ ] No duplicate metadata (each page has unique title/description)

---

## 9. Image & Font Optimization

### Images — `next/image`

- **Always use `<Image>` from `next/image`** — never plain `<img>`
- Automatic: lazy loading, responsive sizing, format conversion (WebP/AVIF), CDN optimization
- **Always provide `width` + `height`** or use `fill` — prevents Cumulative Layout Shift (CLS)
- Use `priority` on above-the-fold hero images (LCP optimization)
- Use `sizes` prop for responsive images — tells the browser which size to download

```tsx
<Image
  src="/hero.jpg"
  alt="Club rankings"
  width={1200}
  height={600}
  priority           // LCP image
  sizes="(max-width: 768px) 100vw, 50vw"
/>
```

### Fonts — `next/font`

- **Use `next/font/google` or `next/font/local`** — zero layout shift, no external network requests
- Fonts are self-hosted at build time (even Google Fonts)
- **Subset fonts** to include only needed character sets
- Use `font-display: 'swap'` (default) — text visible immediately with fallback font

```tsx
import { Inter } from 'next/font/google'
const inter = Inter({ subsets: ['latin'] })
```

### Best Practices

- **Preload critical fonts** — `next/font` handles this automatically
- **Limit font families**: 1-2 max — each adds to bundle
- **Use `placeholder="blur"`** for images with known dimensions — smooth loading
- **External images**: Add domains to `images.remotePatterns` in `next.config.js`

### Audit Checklist

- [ ] All images use `next/image` (no plain `<img>` tags)
- [ ] Images have explicit `width`/`height` or `fill` (no CLS)
- [ ] LCP images have `priority` prop
- [ ] Responsive images use `sizes` prop
- [ ] Fonts loaded via `next/font` (no external stylesheet links)
- [ ] Font families limited to 1-2
- [ ] External image domains configured in `next.config.js`

---

## 10. Internationalization (i18n)

### next-intl (Recommended Library)

The leading i18n solution for Next.js App Router with 930k+ weekly downloads.

### Architecture

```
app/
  [locale]/
    layout.tsx          <- wraps all localized pages
    page.tsx
    (main)/
      rankings/page.tsx
messages/
  de.json               <- German translations
  en.json               <- English translations
```

### Key Patterns

- **Routing**: `[locale]` dynamic segment at the top of the route tree
- **Server Components**: Use `getTranslations()` for async server-side translation
- **Client Components**: Use `useTranslations()` hook
- **Static rendering**: Call `setRequestLocale(locale)` in every Server Component/layout to enable static generation
- **Navigation**: Use `next-intl`'s `<Link>`, `useRouter`, `redirect` — not Next.js built-ins — to preserve locale in URLs

### Best Practices

- **Centralize routing config** in a `routing.ts` file — define supported locales, default locale, custom path mappings
- **Type-safe message keys**: Use next-intl's TypeScript integration for compile-time key checking
- **ICU message format** for plurals, dates, numbers — don't build custom formatting
- **Separate translation files per locale** — don't inline strings in components
- **Proxy/middleware handles locale detection**: Redirect to default locale if none specified

### Audit Checklist

- [ ] All user-facing text goes through translation functions (no hardcoded strings in components)
- [ ] `setRequestLocale()` called in Server Components for static rendering
- [ ] Navigation uses `next-intl` helpers (not Next.js `<Link>` directly)
- [ ] Translation keys are type-safe
- [ ] ICU format used for plurals and formatted values
- [ ] Locale detection handled in proxy

---

## 11. Environment Variables & Configuration

### Variable Types

| Prefix | Accessible in | Bundled | Use for |
|--------|--------------|---------|---------|
| `NEXT_PUBLIC_` | Server + Client | Yes (inlined at build) | Public config (API base URL, feature flags) |
| (no prefix) | Server only | No | Secrets (DB URL, API keys, SMTP credentials) |

### File Hierarchy

```
.env                  <- default for all environments
.env.local            <- local overrides (gitignored)
.env.development      <- `next dev` only
.env.production       <- `next build` / `next start` only
.env.test             <- test environment
```

`.env.local` always overrides others. Never commit it.

### Best Practices

- **Never prefix secrets with `NEXT_PUBLIC_`** — they get inlined into the client bundle
- **Use `server-only` package** for modules that read secrets — prevents accidental client import
- **Validate env vars at startup**: Use `zod` to parse `process.env` in a central config file
- **Runtime env vars** (Docker): Use Server Components + App Router to read `process.env` at request time (not inlined at build)
- **Don't use `next.config.js` `env` object** — it inlines values at build time like `NEXT_PUBLIC_`

### Environment Validation Pattern

```ts
// lib/env.ts
import { z } from 'zod'

const envSchema = z.object({
  DATABASE_URL: z.string().url(),
  SMTP_HOST: z.string(),
  APP_URL: z.string().url(),
})

export const env = envSchema.parse(process.env)
```

### Audit Checklist

- [ ] No secrets exposed via `NEXT_PUBLIC_` prefix
- [ ] `.env.local` is gitignored
- [ ] Environment variables validated at startup (not just `process.env.X!`)
- [ ] `server-only` package used in modules that access secrets
- [ ] All required env vars documented (README or `.env.example`)

---

## 12. Security

### Content Security Policy (CSP)

- Configure via `next.config.js` headers or proxy
- Use **nonces** for inline scripts — generated fresh per request
- Block `unsafe-inline` and `unsafe-eval` in production
- Allow only trusted script sources

### Server Action Security

- **Validate input**: zod/valibot schema on every action
- **Check authorization**: Don't trust that the user has permission just because they reached the action
- **CSRF is handled**: POST-only + Origin/Host comparison + SameSite cookies
- **Rate limit sensitive actions**: Login, password reset, email sending — use `@arcjet/next` or similar
- **Never trust closured-over values**: Server Actions can capture variables from their scope — treat all values from the client as untrusted

### Route Handler Security

- **Webhook signature verification**: Always verify `X-Hub-Signature` or equivalent
- **CORS headers**: Only if needed for external API consumers
- **Rate limiting**: Apply per-IP limits on public endpoints

### Data Exposure Prevention

- **`server-only` module marker**: Import in server-only files — build fails if imported client-side
- **Never pass secrets as props**: Server -> Client serialization exposes them in HTML
- **Taint API** (`experimental_taintObjectReference`): Mark objects that should never cross the server/client boundary

### Headers

```ts
// next.config.js
const securityHeaders = [
  { key: 'X-Frame-Options', value: 'DENY' },
  { key: 'X-Content-Type-Options', value: 'nosniff' },
  { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
  { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
]
```

### Known Vulnerabilities

- **CVE-2025-29927** (March 2025): Middleware/proxy auth bypass — fixed in Next.js 15.1.4+
- Always keep Next.js updated to latest patch version

### Audit Checklist

- [ ] CSP headers configured (at least X-Frame-Options, X-Content-Type-Options)
- [ ] Server Actions validate input and check authorization
- [ ] Rate limiting on auth-related endpoints
- [ ] `server-only` used in modules accessing secrets
- [ ] No sensitive data in Server -> Client Component props
- [ ] Security headers set in `next.config.js`
- [ ] Next.js version is patched (15.1.4+ / latest 16.x)

---

## 13. Error Handling

### File Conventions

| File | Scope | Catches |
|------|-------|---------|
| `error.tsx` | Route segment | Errors in `page.tsx` and children |
| `global-error.tsx` | Entire app | Errors in root `layout.tsx` |
| `not-found.tsx` | Route segment | `notFound()` calls |

### `error.tsx` Best Practices

```tsx
'use client'  // Must be a Client Component

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    // Log to external error service
    reportError(error)
  }, [error])

  return (
    <div>
      <h2>Something went wrong</h2>
      <button onClick={reset}>Try again</button>
    </div>
  )
}
```

### Key Rules

- **`error.tsx` must be `"use client"`** — it's a Client Component
- **`error.tsx` catches errors in `page.tsx`**, not in the `layout.tsx` at the same level
- **`global-error.tsx`** is the only way to catch root layout errors — must render its own `<html>` and `<body>`
- **`redirect()` throws** — never call inside try/catch or the redirect gets swallowed
- **`notFound()` throws** — same rule, don't wrap in try/catch

### Error Reporting

- **Use `digest`** property to match client-side errors with server logs
- **Report to external service** (Sentry, Vercel Analytics) in `useEffect`
- **Never show raw error messages** to users — they may contain sensitive info

### Recovery Patterns

- **`reset()` function**: Re-renders the error boundary's children — useful for transient errors
- **Navigation**: Provide a link to go home or back
- **Retry with backoff**: For network errors, offer a retry that waits before retrying

### Audit Checklist

- [ ] `error.tsx` at key route segments (not just root)
- [ ] `global-error.tsx` exists for root layout errors
- [ ] `not-found.tsx` at dynamic route segments
- [ ] Error boundaries provide recovery actions (reset, navigate)
- [ ] Errors logged to external service
- [ ] `redirect()` and `notFound()` never called inside try/catch
- [ ] No raw error messages exposed to users

---

## 14. Deployment & Production

### Vercel (Recommended)

- Zero-config for Next.js — auto-detects settings
- Edge Functions for proxy, Serverless Functions for API/pages
- Built-in: image optimization, analytics, speed insights, preview deployments
- Set env vars in Vercel dashboard (not committed to git)

### Turbopack (Default in Next.js 16)

- **Turbopack is the default bundler** for both dev and production builds
- File system caching is stable and enabled by default in 16.1 — faster incremental builds
- Opt back into Webpack if needed: `next dev --webpack` / `next build --webpack`
- Experimental bundle analysis: `next experimental-analyze`
- Debug server-side code: `next dev --inspect`

### Self-Hosted

- Enable **standalone output**: `output: 'standalone'` in `next.config.ts`
- Results in minimal `node_modules` — only runtime dependencies
- Use Docker with multi-stage builds for small images
- Share cache across instances: `use cache: remote` with a custom cache handler (Redis, S3)

### Removed Features in Next.js 16

- **AMP support**: Removed entirely
- **`next lint`**: Removed — use ESLint directly (`eslint .`)
- **`serverRuntimeConfig` / `publicRuntimeConfig`**: Removed — use `process.env` in Server Components
- **`next/legacy/image`**: Deprecated — use `next/image` only

### Production Checklist

- **Bundle analysis**: Run `next experimental-analyze` or `@next/bundle-analyzer` to identify bloat
- **No barrel files**: `index.ts` re-exports prevent tree shaking — import from specific files
- **Code splitting**: Next.js auto-splits by route — verify with bundle analyzer
- **Health checks**: Create `/api/health` endpoint that verifies DB connectivity
- **Monitoring**: Track Core Web Vitals (LCP, INP, CLS) with real user data
- **Error tracking**: Sentry or similar for production error alerts

### Performance Optimization

- **Minimize client JS**: Server Components eliminate JS for static content
- **Dynamic imports**: `next/dynamic` for heavy client components (editors, charts, maps)
- **Preload critical resources**: `<link rel="preload">` for fonts, LCP images
- **Compression**: Vercel handles this; self-hosted needs gzip/brotli config

### Audit Checklist

- [ ] Bundle analyzed for unexpected large dependencies
- [ ] No barrel files causing bundle bloat
- [ ] Dynamic imports for heavy client-only components
- [ ] Health check endpoint exists
- [ ] Core Web Vitals monitored
- [ ] Error tracking service configured
- [ ] Environment variables set in deployment platform (not in code)
- [ ] Standalone output enabled if self-hosting
- [ ] Turbopack used (default in 16) — no unnecessary Webpack fallback

---

## 15. next.config.js Patterns

### Essential Configuration

```ts
// next.config.ts (TypeScript config recommended in Next.js 16)
import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
  // Standalone output for Docker/self-hosting
  output: 'standalone',

  // React Compiler (stable — top-level config, no longer experimental)
  reactCompiler: true,

  // Cache Components (opt-in static + dynamic hybrid)
  cacheComponents: true,

  // Image optimization domains
  images: {
    remotePatterns: [
      { protocol: 'https', hostname: 'example.com' },
    ],
  },

  // Security headers
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: securityHeaders,
      },
    ]
  },

  // Redirects (permanent = 308)
  async redirects() {
    return [
      { source: '/old-path', destination: '/new-path', permanent: true },
    ]
  },
}
```

### Things to Avoid

- **Don't disable TypeScript checks** (`ignoreBuildErrors`) in production — fix the errors instead
- **Don't disable ESLint** (`ignoreDuringBuilds`) in production
- **Don't use `env` object** for secrets — use `process.env` in server code
- **Don't use `images.domains`** — use `images.remotePatterns` instead (domains is deprecated)
- **Don't use `experimental.reactCompiler`** — moved to top-level `reactCompiler` in Next.js 16
- **Don't use `experimental.ppr`** — replaced by `cacheComponents` in Next.js 16
- **Don't configure webpack** unless absolutely necessary — Turbopack is the default bundler

### Audit Checklist

- [ ] TypeScript errors not ignored in production builds
- [ ] ESLint not ignored in production builds
- [ ] Security headers configured
- [ ] Image remote patterns configured (not deprecated `domains`)
- [ ] React Compiler enabled via top-level `reactCompiler` config
- [ ] Using `next.config.ts` (TypeScript) instead of `.js`/`.mjs`
- [ ] No unnecessary webpack/Turbopack customization

---

## Sources

- [Next.js 16 Blog Post](https://nextjs.org/blog/next-16)
- [Next.js 16.1 Blog Post](https://nextjs.org/blog/next-16-1)
- [Next.js 16 Upgrade Guide](https://nextjs.org/docs/app/guides/upgrading/version-16)
- [Next.js Caching & Revalidation](https://nextjs.org/docs/app/getting-started/caching-and-revalidating)
- [Next.js `use cache` Directive](https://nextjs.org/docs/app/api-reference/directives/use-cache)
- [Next.js `cacheLife` Function](https://nextjs.org/docs/app/api-reference/functions/cacheLife)
- [Next.js Proxy Convention](https://nextjs.org/docs/app/api-reference/file-conventions/proxy)
- [Next.js Server & Client Components](https://nextjs.org/docs/app/getting-started/server-and-client-components)
- [Next.js Data Fetching Patterns](https://nextjs.org/docs/app/building-your-application/data-fetching/patterns)
- [Next.js Error Handling](https://nextjs.org/docs/app/getting-started/error-handling)
- [Next.js Metadata & OG Images](https://nextjs.org/docs/app/getting-started/metadata-and-og-images)
- [Next.js Image Optimization](https://nextjs.org/docs/app/getting-started/images)
- [Next.js Font Optimization](https://nextjs.org/docs/app/getting-started/fonts)
- [Next.js Parallel Routes](https://nextjs.org/docs/app/api-reference/file-conventions/parallel-routes)
- [Next.js Intercepting Routes](https://nextjs.org/docs/app/api-reference/file-conventions/intercepting-routes)
- [Next.js Route Handlers Best Practices](https://makerkit.dev/blog/tutorials/nextjs-api-best-practices)
- [Next.js Security Guide 2025](https://www.turbostarter.dev/blog/complete-nextjs-security-guide-2025-authentication-api-protection-and-best-practices)
- [Next.js Production Checklist](https://nextjs.org/docs/app/guides/production-checklist)
- [next-intl Getting Started](https://next-intl.dev/docs/getting-started/app-router)
- [ISR Guide](https://nextjs.org/docs/app/guides/incremental-static-regeneration)
- [Data Security Guide](https://nextjs.org/docs/app/guides/data-security)
- [How to Think About Security in Next.js](https://nextjs.org/blog/security-nextjs-server-components-actions)
