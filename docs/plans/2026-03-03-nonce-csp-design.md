# Nonce-based Content Security Policy

## Problem

The production CSP uses a single SHA-256 hash for the theme detection script, but Next.js App Router generates many inline `<script>` tags for RSC hydration. These scripts have different hashes per build, so the static hash approach blocks them all — breaking the app entirely.

## Design

### `proxy.ts` — nonce generation

Create `web/proxy.ts` (Next.js 16's replacement for `middleware.ts`) that:

1. Generates a random nonce per request via `crypto.randomUUID()`
2. Builds a CSP header with `'nonce-<value>'` in `script-src`
3. Sets both `x-nonce` request header and `Content-Security-Policy` response header
4. Uses a matcher to skip static assets and prefetches

Key CSP directives:
- `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'` — nonce allows framework scripts, `strict-dynamic` allows scripts loaded by nonced scripts
- `style-src 'self' 'unsafe-inline'` — required for Tailwind/Next.js style injection
- `connect-src 'self'` — restricts fetch/XHR to same origin
- `form-action 'self'` — restricts form submissions
- `frame-ancestors 'none'` — prevents framing (clickjacking)
- Dev mode adds `'unsafe-eval'` for React debugging

### `layout.tsx` — nonce on theme script

Read the nonce from `headers()` and pass it to the inline theme detection `<script>` tag via the `nonce` attribute. Next.js automatically applies the nonce to all framework-generated scripts.

### `next.config.ts` — remove static CSP

Remove the `Content-Security-Policy` header from `next.config.ts` `headers()` since `proxy.ts` now handles it dynamically. Keep the other security headers (HSTS, X-Frame-Options, etc.) in `next.config.ts`.

## Files changed

1. `web/proxy.ts` — new file, nonce generation + CSP header
2. `web/app/layout.tsx` — read nonce from headers, pass to theme script
3. `web/next.config.ts` — remove CSP from static headers, remove themeScriptHash

## Performance impact

None. The app is already dynamically rendered (reads cookies, locale per request). The nonce adds ~0.1ms of overhead per request.
