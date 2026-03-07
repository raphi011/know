# SSE Change Events Design

## Goal

Keep the web frontend automatically in sync with all document mutations (agent, MCP, CLI, manual save) via server-sent events, replacing scattered `router.refresh()` calls with a unified event-driven system.

## Architecture

```
CLI ─────┐
MCP ─────┤──→ document.Service ──→ event.Bus ──→ SSE /events ──→ browser
Agent ───┤        ↕
Web UI ──┘     SurrealDB
```

All document mutations flow through `document.Service`, which publishes to an in-process event bus. An SSE endpoint streams events to connected browsers. The frontend reacts by refreshing server components and handling editor conflicts.

## Event Format

Generic envelope with type-specific payload:

```go
type ChangeEvent struct {
    Type    string `json:"type"`    // "document.created", "document.updated", etc.
    VaultID string `json:"vaultId"`
    Payload any    `json:"payload"`
}

type DocumentPayload struct {
    DocID       string `json:"docId"`
    Path        string `json:"path"`
    OldPath     string `json:"oldPath,omitempty"` // only for document.moved
    ContentHash string `json:"contentHash"`
}
```

Event types: `document.created`, `document.updated`, `document.deleted`, `document.moved`.

Extensible — future event types (e.g. `task.created`) add new payload structs without changing the envelope.

## Backend Components

### Event Bus (`internal/event/bus.go`)

In-process pub/sub. Subscribers register per vault, receive only events for their vault.

- `Subscribe(vaultID) → (<-chan ChangeEvent, unsubscribe func())`
- `Publish(event)` — fans out to all subscribers for that vault
- Buffered channels (64) with slow-consumer protection (close channel if full)
- `document.Service` gets an optional `*event.Bus` (nil-safe)

### SSE Handler (`internal/event/handler.go`)

`GET /events?vaultId=...` — long-lived SSE connection.

- Auth middleware (same as other endpoints)
- Vault access check
- Subscribes to event bus for the requested vault
- Streams events as `data: {json}\n\n`
- Keepalive `: ping\n\n` every 30s
- Cleans up subscription on client disconnect

### Publishing in `document.Service`

After each mutation (Create, Update, Delete, Move, DeleteByPrefix, MoveByPrefix, Rollback), publish the corresponding event. Nil bus is a no-op.

## Frontend Components

### SSE Proxy (`web/app/api/events/route.ts`)

Next.js API route that proxies the SSE stream from the Go backend. Reads session cookie, forwards auth token. Same pattern as the agent chat proxy.

### Hook: `useChangeEvents(vaultId)` (`web/hooks/use-change-events.ts`)

Opens `EventSource` to `/api/events?vaultId=...`. Dispatches actions based on event type:

| Event | Action |
|-------|--------|
| `document.created` | `router.refresh()` |
| `document.updated` | `router.refresh()` + notify editor if doc is open |
| `document.deleted` | `router.refresh()` + redirect if deleted doc is open |
| `document.moved` | `router.refresh()` + update URL if moved doc is open |

Mounted in `AppShellWrapper` — always active when authenticated.

### Editor Conflict Detection (`DocumentEditor`)

Tracks `isDirty` (user has unsaved local edits) and `contentHash` (DB state at page load).

When `document.updated` arrives for the open document:

| User has unsaved edits? | Behavior |
|------------------------|----------|
| No | Silently remount editor with fresh content (`key={contentHash}`) |
| Yes | Show conflict banner |

**Self-change filtering**: After saving, the editor knows the hash it wrote. If the event's `contentHash` matches, it's our own save — ignore it.

### External Change Banner (`web/components/domain/external-change-banner.tsx`)

Non-blocking banner: "This document was updated externally."

Three actions:
- **Keep my edits** — dismiss, continue editing. Next save overwrites external change.
- **Load new version** — discard local edits, remount editor with fresh content.
- **View diff** — show external changes as a diff (reuses existing `DiffHunk` rendering).

## Cleanup

Remove all existing ad-hoc `router.refresh()` calls:
- `hadWriteToolRef` tracking in agent chat context
- Manual refresh calls in `doc-tree.tsx`
- Any other scattered refresh triggers after mutations

All replaced by the event bus.
