# HTMX

[HTMX](https://htmx.org/) enables dynamic interactions by swapping HTML fragments from the server. No client-side JS framework, no JSON APIs for UI — just HTML over the wire.

**Version**: 2.0.4 (bundled at `internal/web/static/js/htmx.min.js`)

## Core Setup

```html
<!-- layout.templ -->
<script src="/static/js/htmx.min.js"></script>
<script src="/static/js/sse.js"></script>
<body hx-boost="true">
```

`hx-boost="true"` on `<body>` intercepts all link clicks and form submissions, loading content via AJAX and swapping the `<body>` — giving SPA-like navigation without JS.

## Attributes Used in This Project

| Attribute | Purpose | Example |
|-----------|---------|---------|
| `hx-get` | Issue GET request | `hx-get="/hx/search"` |
| `hx-post` | Issue POST request | `hx-post="/hx/vault/switch"` |
| `hx-target` | Element to swap content into | `hx-target="#search-results"` |
| `hx-trigger` | Event that fires the request | `hx-trigger="input changed delay:300ms"` |
| `hx-swap` | How to swap content | `hx-swap="innerHTML"` |
| `hx-ext` | Load HTMX extension | `hx-ext="sse"` |
| `sse-connect` | SSE EventSource URL | `sse-connect="/hx/doc/events?..."` |
| `sse-swap` | SSE event name to trigger swap | `sse-swap="doc-updated"` |

## Partial Endpoints

All HTMX endpoints live under `/hx/*` and are session-protected.

| Endpoint | Method | Purpose | Response |
|----------|--------|---------|----------|
| `/hx/search` | GET | Search documents | HTML: result list with snippets |
| `/hx/sidebar` | GET | Refresh folder tree | HTML: sidebar component |
| `/hx/doc/events` | GET | Stream doc changes | SSE: `doc-updated` events with HTML |
| `/hx/versions` | GET | Version history | HTML: version list |
| `/hx/version/diff` | GET | Compare versions | HTML: diff view |
| `/hx/vault/switch` | POST | Change vault | Empty (session side-effect only) |
| `/hx/settings/locale` | POST | Change language | Empty (triggers reload) |
| `/hx/settings/theme` | POST | Change theme | Empty (updates cookie) |

## Key Patterns

### Debounced Search

```html
<input
    hx-get="/hx/search"
    hx-trigger="input changed delay:300ms, search"
    hx-target="#search-results"
    name="q"
/>
<div id="search-results"></div>
```

- `input changed` fires on value change (not every keystroke)
- `delay:300ms` debounces to avoid hammering the server
- `search` is a custom event dispatched by Cmd+K handler in `app.js`

### Lazy Loading

```html
<summary
    hx-get="/hx/versions?doc=abc"
    hx-target="#version-list"
    hx-trigger="click once">
    Version History
</summary>
<div id="version-list"></div>
```

`click once` — fetches on first click, never refetches. Good for content that doesn't change during a page view.

### Side-Effect-Only Requests

```html
<form hx-post="/hx/vault/switch" hx-swap="none">
    <select name="vault">...</select>
    <button type="submit">Switch</button>
</form>
```

`hx-swap="none"` — the server processes the request (updates session), but nothing is swapped in the DOM. The page may reload or redirect separately.

### SSE Live Updates

```html
<div hx-ext="sse" sse-connect="/hx/doc/events?vault=abc&path=/notes/foo">
    <article id="doc-content" sse-swap="doc-updated" hx-swap="innerHTML">
        <!-- auto-updates when server sends event: doc-updated -->
    </article>
</div>
```

See `docs/sse.md` for full SSE details.

## Buffered Rendering

HTMX partial handlers use `renderBuffered()` to prevent partial HTML on errors:

```go
// Renders to buffer first, writes to response only on success
renderBuffered(w, r, partials.SearchResults(items))
```

If the templ component fails to render, the client gets a clean error response instead of corrupt HTML that HTMX would swap into the DOM.

## Best Practices

### HTML First

1. **Semantic HTML** — use `<form>`, `<button>`, `<a>`, `<details>/<summary>` before reaching for HTMX
2. **Progressive enhancement** — pages should work with `hx-boost` alone; HTMX partials add interactivity on top
3. **Server renders HTML** — endpoints return HTML fragments, not JSON; the server owns the rendering

### Request Patterns

4. **Debounce user input** — always use `delay:` on text inputs to avoid excessive requests
5. **`click once` for static content** — prevents unnecessary refetches
6. **`hx-swap="none"` for side effects** — vault switch, locale change, etc.
7. **Target specific elements** — use `hx-target` with IDs; avoid swapping large sections of the page

### Response Patterns

8. **Buffer partial rendering** — always use `renderBuffered()` for HTMX responses
9. **Return appropriate status codes** — HTMX respects HTTP status codes; 4xx/5xx prevent swaps by default
10. **Set `Content-Type: text/html`** — HTMX expects HTML responses

### Avoid

11. **Don't use HTMX for streaming text** — use `fetch()` + `ReadableStream` instead (see `agent.js`)
12. **Don't inline complex JS** — keep onclick handlers simple; move logic to external JS files
13. **Don't fight the browser** — let `hx-boost` handle navigation; don't rebuild routing in JS

## When to Use Raw JS Instead

The agent chat uses `fetch()` + `ReadableStream` in `agent.js` instead of HTMX because:

- It needs streaming text (word-by-word rendering)
- It needs structured JSON events (tool calls, approvals)
- It needs stateful DOM manipulation (append vs replace)

HTMX is best for request → response → swap. For complex streaming or stateful interactions, use vanilla JS.
