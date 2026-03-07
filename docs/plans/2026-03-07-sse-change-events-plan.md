# SSE Change Events Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep the web frontend automatically in sync with all document mutations via server-sent events, with editor conflict detection when the user has unsaved edits.

**Architecture:** An in-process event bus in Go publishes change events from `document.Service`. A new SSE endpoint streams these to the browser. A React hook listens and triggers `router.refresh()` or shows a conflict banner in the editor.

**Tech Stack:** Go (event bus, SSE handler), Next.js API route (SSE proxy), React hook (`EventSource`), `diff` npm package (client-side diff for conflict view)

---

### Task 1: Event Bus

**Files:**
- Create: `internal/event/bus.go`
- Create: `internal/event/bus_test.go`

The event bus is a simple in-process pub/sub. Subscribers register per vault and receive only events for their vault. Buffered channels with slow-consumer protection.

**Types:**

```go
package event

type ChangeEvent struct {
    Type    string `json:"type"`    // "document.created", "document.updated", etc.
    VaultID string `json:"vaultId"`
    Payload any    `json:"payload"`
}

type DocumentPayload struct {
    DocID       string `json:"docId"`
    Path        string `json:"path"`
    OldPath     string `json:"oldPath,omitempty"`
    ContentHash string `json:"contentHash"`
}
```

**Bus API:**

```go
type Bus struct {
    mu   sync.Mutex
    subs map[string]map[uint64]chan ChangeEvent // vaultID → subID → channel
    next uint64
}

func New() *Bus
func (b *Bus) Publish(event ChangeEvent)
func (b *Bus) Subscribe(vaultID string) (ch <-chan ChangeEvent, unsubscribe func())
```

- `Subscribe` creates a buffered channel (capacity 64) and registers it
- `Publish` fans out to all subscribers for the event's VaultID. If a subscriber's channel is full, close it (slow consumer eviction)
- `unsubscribe` removes the subscriber and closes its channel

**Tests (`bus_test.go`):**
1. `TestBus_PublishSubscribe` — subscribe, publish, receive event
2. `TestBus_MultipleSubscribers` — two subscribers both receive the same event
3. `TestBus_VaultIsolation` — subscriber for vault A does NOT receive events for vault B
4. `TestBus_Unsubscribe` — after unsubscribe, channel is closed and no longer receives
5. `TestBus_SlowConsumer` — fill channel to capacity, next publish closes it
6. `TestBus_ConcurrentPublish` — goroutine safety with parallel publishes

**Verify:** `go test -buildvcs=false ./internal/event/...`

---

### Task 2: Wire Event Bus into document.Service

**Files:**
- Modify: `internal/document/service.go` (struct, constructor, mutation methods)
- Modify: `internal/graph/resolver.go` (pass bus to NewService)
- Modify: `cmd/knowhow-server/main.go` (create bus, pass to resolver)

**Changes to `document.Service`:**

Add `bus *event.Bus` field to the Service struct (line 28). Update `NewService` to accept an optional `*event.Bus` parameter. Add a helper method:

```go
func (s *Service) publishDocEvent(eventType string, vaultID string, doc *models.Document) {
    if s.bus == nil {
        return
    }
    docID, _ := models.RecordIDString(doc.ID)
    s.bus.Publish(event.ChangeEvent{
        Type:    eventType,
        VaultID: vaultID,
        Payload: event.DocumentPayload{
            DocID:       docID,
            Path:        doc.Path,
            ContentHash: doc.ContentHash,
        },
    })
}
```

Call `publishDocEvent` at the end of each mutation method:
- `Create` (line ~150): publish `"document.created"` if `created` is true, else `"document.updated"`
- `Delete` (line ~313): publish `"document.deleted"`
- `DeleteByPrefix` (line ~340): publish `"document.deleted"` for each deleted doc
- `Move` (line ~407): publish `"document.moved"` (set `OldPath` in payload)
- `MoveByPrefix` (line ~377): publish `"document.moved"` for each moved doc
- `Rollback` in `version.go` (line ~113): publish `"document.updated"`

**Note:** `Update` (line 288) delegates to `Create`, so it's already covered.

**Wire in resolver.go:**

In `NewResolver` (line 36), create the bus and pass it:

```go
bus := event.New()
docService := document.NewService(dbClient, embedder, chunkConfig, versionConfig, bus)
```

Store `bus` in the `Resolver` struct so the SSE handler can access it. Add a getter:

```go
func (r *Resolver) EventBus() *event.Bus { return r.bus }
```

**Verify:** `just build-all && just test`

---

### Task 3: SSE Handler

**Files:**
- Create: `internal/event/handler.go`
- Modify: `cmd/knowhow-server/main.go` (register route)

**Handler:**

```go
func HandleEvents(bus *Bus) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        vaultID := r.URL.Query().Get("vaultId")
        if vaultID == "" {
            http.Error(w, "vaultId query parameter required", http.StatusBadRequest)
            return
        }

        if _, err := auth.FromContext(r.Context()); err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        if err := auth.RequireVaultAccess(r.Context(), vaultID); err != nil {
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }

        flusher, ok := w.(http.Flusher)
        if !ok {
            http.Error(w, "streaming not supported", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("X-Accel-Buffering", "no")

        ch, unsub := bus.Subscribe(vaultID)
        defer unsub()

        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case evt, ok := <-ch:
                if !ok {
                    return // channel closed (slow consumer or shutdown)
                }
                data, err := json.Marshal(evt)
                if err != nil {
                    slog.Warn("failed to marshal change event", "error", err)
                    continue
                }
                fmt.Fprintf(w, "data: %s\n\n", data)
                flusher.Flush()
            case <-ticker.C:
                fmt.Fprintf(w, ": ping\n\n")
                flusher.Flush()
            case <-r.Context().Done():
                return
            }
        }
    }
}
```

**Register in main.go** (after the existing agent routes):

```go
mux.Handle("/events", authMw(event.HandleEvents(resolver.EventBus())))
```

**Verify:** `just build-all` — can manually test with `curl -N -H "Authorization: Bearer $TOKEN" "http://localhost:8484/events?vaultId=default"`

---

### Task 4: Next.js SSE Proxy Route

**Files:**
- Create: `web/app/api/events/route.ts`

Follow the exact pattern from `web/app/api/agent/chat/route.ts` but use GET instead of POST, and forward `vaultId` as a query parameter.

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/app/lib/session";
import { getActiveConnection } from "@/app/lib/actions/connections";
import { env } from "@/app/lib/env";

export async function GET(request: NextRequest) {
  let backendUrl: string;
  let authHeader: Record<string, string> = {};

  if (env.AUTH_DISABLED) {
    backendUrl = env.BACKEND_URL;
  } else {
    const session = await getSession();
    if (!session || session.servers.length === 0) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }
    const conn = await getActiveConnection();
    if (!conn) {
      return NextResponse.json({ error: "No server configured" }, { status: 503 });
    }
    backendUrl = conn.url;
    authHeader = { Authorization: `Bearer ${conn.token}` };
  }

  const vaultId = request.nextUrl.searchParams.get("vaultId");
  if (!vaultId) {
    return NextResponse.json({ error: "vaultId required" }, { status: 400 });
  }

  let upstream: Response;
  try {
    upstream = await fetch(
      `${backendUrl}/events?vaultId=${encodeURIComponent(vaultId)}`,
      { headers: { ...authHeader } },
    );
  } catch (err) {
    console.error("Events API unreachable:", err);
    return NextResponse.json({ error: "Events API unreachable" }, { status: 502 });
  }

  if (!upstream.ok) {
    const text = await upstream.text().catch(() => "Unknown error");
    return NextResponse.json({ error: text }, { status: upstream.status });
  }

  if (!upstream.body) {
    return NextResponse.json({ error: "No response body" }, { status: 502 });
  }

  const stream = new ReadableStream({
    async start(controller) {
      const reader = upstream.body!.getReader();
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          controller.enqueue(value);
        }
      } catch {
        // Client disconnected
      } finally {
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
```

**Verify:** `cd web && bun run typecheck`

---

### Task 5: Frontend useChangeEvents Hook

**Files:**
- Create: `web/hooks/use-change-events.ts`
- Modify: `web/app/(main)/app-shell-wrapper.tsx` (mount hook)

**Hook:**

```typescript
"use client";

import { useEffect, useRef } from "react";
import { useRouter, usePathname } from "next/navigation";

export type DocumentChangePayload = {
  docId: string;
  path: string;
  oldPath?: string;
  contentHash: string;
};

export type ChangeEvent = {
  type: string;
  vaultId: string;
  payload: DocumentChangePayload;
};

type ChangeHandler = (event: ChangeEvent) => void;

export function useChangeEvents(
  vaultId: string | null,
  onDocumentChange?: ChangeHandler,
) {
  const router = useRouter();
  const pathname = usePathname();
  const pathnameRef = useRef(pathname);
  pathnameRef.current = pathname;

  const handlerRef = useRef(onDocumentChange);
  handlerRef.current = onDocumentChange;

  useEffect(() => {
    if (!vaultId) return;

    const es = new EventSource(`/api/events?vaultId=${encodeURIComponent(vaultId)}`);

    es.onmessage = (msg) => {
      if (!msg.data || msg.data.trim() === "") return;
      try {
        const event: ChangeEvent = JSON.parse(msg.data);

        if (event.type.startsWith("document.")) {
          // Refresh server components (sidebar, etc.)
          router.refresh();

          // Notify editor if callback provided
          handlerRef.current?.(event);

          // Handle deletion of currently open doc
          if (event.type === "document.deleted") {
            const openPath = pathnameRef.current.startsWith("/docs/")
              ? "/" + pathnameRef.current.slice("/docs/".length)
              : null;
            if (openPath === event.payload.path) {
              router.push("/docs");
            }
          }

          // Handle move of currently open doc
          if (event.type === "document.moved" && event.payload.oldPath) {
            const openPath = pathnameRef.current.startsWith("/docs/")
              ? "/" + pathnameRef.current.slice("/docs/".length)
              : null;
            if (openPath === event.payload.oldPath) {
              router.push(`/docs${event.payload.path}`);
            }
          }
        }
      } catch (err) {
        console.error("Failed to parse change event:", err);
      }
    };

    es.onerror = () => {
      // EventSource auto-reconnects. Nothing to do.
    };

    return () => es.close();
  }, [vaultId, router]);
}
```

**Mount in `app-shell-wrapper.tsx`:**

Import the hook and call it inside `AppShellWrapper`:

```typescript
import { useChangeEvents } from "@/hooks/use-change-events";

// Inside the component, after existing hooks:
useChangeEvents(vault?.id ?? null);
```

The `onDocumentChange` callback is not wired yet — that comes in Task 7 (editor conflict detection).

**Verify:** `cd web && bun run typecheck` — and manually test: start the app, open the sidebar, create a document via CLI or MCP, confirm the sidebar updates.

---

### Task 6: Extract Shared Diff Components

**Files:**
- Create: `web/components/domain/diff-view.tsx` (extract `HunkView` + `LineView`)
- Modify: `web/components/domain/version-diff-view.tsx` (import from shared)
- Modify: `web/components/domain/tool-approval-card.tsx` (import from shared, if applicable)

Extract `HunkView` and `LineView` from `version-diff-view.tsx` (lines 113-156) into a new shared component file `diff-view.tsx`. These render diff hunks with colored add/delete/context lines and will be reused by the conflict banner.

Export:
- `HunkView` component
- `LineView` component

Update `version-diff-view.tsx` to import from `diff-view.tsx` instead of defining them locally.

Check if `tool-approval-card.tsx` has its own inline diff rendering that could also use the shared components — if so, update it too.

**Verify:** `cd web && bun run typecheck`

---

### Task 7: Editor Conflict Detection + Banner

**Files:**
- Modify: `web/components/domain/document-editor.tsx`
- Create: `web/components/domain/external-change-banner.tsx`
- Modify: `web/app/(main)/docs/[...path]/page.tsx` (add `key` prop)
- Modify: `web/messages/en.json` (add i18n keys)
- Modify: `web/messages/de.json` (add i18n keys)

Install `diff` package for client-side diff computation:

```bash
cd web && bun add diff && bun add -d @types/diff
```

**i18n keys** (add to `docs` section in both locale files):

```json
"externalChange": "Document updated externally",
"externalChangeDesc": "This document was modified by another source while you were editing.",
"keepEdits": "Keep my edits",
"loadNewVersion": "Load new version",
"viewDiff": "View changes"
```

**page.tsx change:**

Add `key={document.contentHash}` to `<DocumentEditor>`. This causes React to remount the editor when the content hash changes (from `router.refresh()`), which handles the case where the user has NO unsaved edits — the editor simply remounts with fresh data.

```tsx
<DocumentEditor
  key={document.contentHash}
  document={document}
  vaultId={vault.id}
  versions={versionData.versions}
  versionsTotalCount={versionData.totalCount}
/>
```

**DocumentEditor changes:**

The editor needs to detect external changes when it has unsaved edits. The flow:

1. Track `lastSavedHash` — set to `document.contentHash` on mount, updated after each successful save
2. Receive external change events via a callback from `useChangeEvents`
3. When an event arrives for this document's path:
   - If `event.payload.contentHash === lastSavedHash` → it's our own save, ignore
   - If `status === "idle" || status === "saved"` → no unsaved edits, `key` remount handles it
   - If status is `"unsaved"` → show conflict banner with the new `contentHash`

The `AppShellWrapper` needs to pass the `onDocumentChange` callback down. Since the editor is a server component's child, we'll use a context or expose the callback from `useChangeEvents`. Simplest approach: make `useChangeEvents` store the latest change event in state and expose it, then the editor reads it.

Better approach: create a `ChangeEventsContext` that provides the latest document change event. The hook publishes to the context, the editor subscribes.

**Create `web/hooks/use-change-events.ts` context extension:**

Update the hook to also provide a context. The `AppShellWrapper` renders `<ChangeEventsProvider>`, and the editor uses `useLatestDocumentChange(path)` to get notified.

**External Change Banner (`external-change-banner.tsx`):**

```typescript
type ExternalChangeBannerProps = {
  onKeepEdits: () => void;
  onLoadNew: () => void;
  onViewDiff: () => void;
  showDiff: boolean;
  diffHunks?: { header: string; lines: { type: string; content: string }[] }[];
};
```

Renders an amber/warning banner with three buttons. When "View diff" is clicked, expands to show the diff using the shared `HunkView`/`LineView` components.

The diff is computed client-side using the `diff` package:
- `structuredPatch(oldContent, newContent)` → produces hunks
- Map to the `DiffHunk`/`DiffLine` shape for rendering

**Verify:** `cd web && bun run typecheck`

---

### Task 8: Remove Ad-Hoc router.refresh() Calls

**Files:**
- Modify: `web/components/domain/agent-chat-context.tsx` (remove hadWriteToolRef, router import if unused)
- Modify: `web/components/doc-tree.tsx` (remove router.refresh() calls)
- Modify: `web/components/domain/document-history.tsx` (remove router.refresh() call)

**agent-chat-context.tsx removals:**
- Line 3: Remove `useRouter` import (if no other usage remains)
- Line 134: Remove `const hadWriteToolRef = useRef(false);`
- Line 132: Remove `const router = useRouter();`
- Lines 346-348: Remove the `if (event.tool === "create_document" ...)` block in tool_start
- Line 290: Remove `hadWriteToolRef.current = false;` in doStream
- Lines 420-423: Remove the `if (hadWriteToolRef.current)` block in msg_end

**doc-tree.tsx removals:**
- Remove all `router.refresh()` calls (lines 253, 343, 363, 374, 395, 410, 423)
- These are after document create, folder create, document delete, document move, bulk operations, and file import
- Keep the `router` import if it's used for `router.push()` navigation

**document-history.tsx:**
- Remove `router.refresh()` call (line 48) after rollback

**Important:** Do NOT remove `router.refresh()` calls in `vault-switcher.tsx` — vault changes are not covered by the document event bus.

**Verify:** `cd web && bun run typecheck && just test`

---

### Task 9: Update README

**Files:**
- Modify: `README.md`

Add a brief section about real-time updates after the "Agent Tool Approval" section:

```markdown
## Real-Time Updates

The web frontend stays automatically in sync with all document changes via Server-Sent Events.
Any document mutation — from the editor, agent chat, MCP tools, or CLI scraping — triggers
an immediate update in all connected browsers.

- **Sidebar** updates automatically when documents are created, deleted, or moved
- **Editor** detects external changes while you're editing and shows a conflict banner
  with options to keep your edits, load the new version, or view the diff
- **Navigation** updates automatically when the open document is moved or deleted
```

**Verify:** `just build-all && just test`
