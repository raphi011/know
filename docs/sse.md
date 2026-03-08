# Server-Sent Events (SSE)

SSE provides real-time server-to-client streaming over HTTP. Unlike WebSockets, SSE is unidirectional (server → client), uses standard HTTP, and reconnects automatically.

## Protocol Format

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no

event: doc-updated
data: <html content here>

data: keepalive

```

- Each message ends with `\n\n` (double newline)
- `event:` names the event type (default: `message`)
- `data:` carries the payload (can span multiple lines)
- `id:` sets the last-event-ID for reconnection
- `retry:` tells the client how long to wait before reconnecting (ms)

## Event Bus Architecture

**File**: `internal/event/bus.go`

In-process pub/sub with vault-scoped routing:

```
Publisher → Bus → [vault:abc subscribers]
                → [vault:xyz subscribers]
```

- **Channels**: buffered at 64 events per subscriber
- **Slow consumer eviction**: if a subscriber's channel fills up, it's closed and removed — prevents one slow client from blocking the bus
- **Thread-safe**: `sync.RWMutex` protects the subscriber map
- **Unsubscribe**: returns a `func()` (safe to call multiple times via `sync.Once`)

### Event Types

| Event | Payload | When |
|-------|---------|------|
| `document.created` | `DocumentPayload{DocID, Path, ContentHash}` | New document saved |
| `document.updated` | `DocumentPayload{DocID, Path, ContentHash}` | Content changed |
| `document.deleted` | `DocumentPayload{DocID, Path}` | Document removed |
| `document.moved` | `DocumentPayload{DocID, Path, OldPath}` | Path changed |

### Path-Scoped Filtering

`SubscribeByPath(vaultID, docPath)` wraps `Subscribe()` and filters events to only those matching the given path (checks both `Path` and `OldPath` for move events). This prevents unnecessary DOM updates when viewing a specific document.

## SSE Endpoints

| Endpoint | Handler | Purpose |
|----------|---------|---------|
| `GET /events?vaultId=...` | `event.HandleEvents()` | Raw vault events (JSON payload) |
| `GET /hx/doc/events?vault=...&path=...` | `web.handleDocEvents()` | Path-scoped, returns HTML fragments for HTMX swap |

### Required HTTP Headers

```go
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")
w.Header().Set("Connection", "keep-alive")
w.Header().Set("X-Accel-Buffering", "no") // Prevents nginx/reverse proxy buffering
```

### Keepalive

Send a comment or empty data line every 30 seconds to prevent proxies/load balancers from closing idle connections:

```
data: \n\n
```

## HTMX SSE Extension

**File**: `internal/web/static/js/sse.js`

The HTMX SSE extension connects SSE streams to DOM updates:

```html
<div hx-ext="sse" sse-connect="/hx/doc/events?vault=abc&path=/notes/foo">
  <article id="doc-content" sse-swap="doc-updated" hx-swap="innerHTML">
    <!-- content auto-updates when server sends event: doc-updated -->
  </article>
</div>
```

- `sse-connect` — URL for the EventSource
- `sse-swap` — event name that triggers a DOM swap
- Auto-reconnect with exponential backoff (doubles each retry, max 128s)
- Cleanup: EventSource closed when element is removed from DOM

## Best Practices

### Server Side

1. **Always use `X-Accel-Buffering: no`** — reverse proxies (nginx, Caddy) buffer responses by default, which breaks streaming
2. **Flush after each event** — call `flusher.Flush()` after writing each SSE message
3. **Detect client disconnect** — use `r.Context().Done()` to stop the goroutine when the client disconnects
4. **Buffered rendering for HTML partials** — render templ components to a buffer before writing to the SSE stream; prevents partial/corrupt HTML if rendering fails mid-write
5. **Scope events narrowly** — use path filtering to avoid sending events the client doesn't need
6. **Handle slow consumers** — cap channel buffers and evict slow subscribers rather than blocking publishers

### Client Side

1. **Use the HTMX SSE extension** for DOM updates — don't write custom EventSource JS unless you need streaming text (like the agent chat)
2. **EventSource reconnects automatically** — browsers reconnect on disconnect; the SSE extension adds exponential backoff
3. **Unique element IDs** — `sse-swap` targets must have stable IDs for swap to work correctly
4. **`hx-swap="innerHTML"`** — most common swap strategy for SSE; replaces content without touching the container element

### When NOT to Use SSE

- **Bidirectional communication** — use WebSockets instead
- **Binary data** — SSE is text-only (UTF-8)
- **High-frequency updates** (>10/sec) — consider WebSockets or batching
- **Large payloads** — SSE messages should be small; for large data, send a notification via SSE and let the client fetch the full data

## Flow: Live Document Updates

```
1. Browser loads DocViewPage → templ renders with sse-connect="/hx/doc/events?..."
2. HTMX SSE extension creates EventSource → server calls bus.SubscribeByPath()
3. Document is edited (CLI, WebDAV, other browser tab)
4. document.Service publishes ChangeEvent to bus
5. Bus fans out to all vault subscribers
6. handleDocEvents() filters by path, re-renders markdown, sends:
   event: doc-updated
   data: <article>...rendered HTML...</article>
7. HTMX swaps innerHTML of #doc-content
```
