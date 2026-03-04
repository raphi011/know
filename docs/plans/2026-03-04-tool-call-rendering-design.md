# Improved Tool Call Rendering

## Context

Tool calls in the agent chat currently render as flat amber badges with minimal info ("Searching: query", "Found 3"). No spinner for inflight calls, no structured metadata (timing, doc count, content size), and no way to see matched documents. This plan adds structured metadata throughout the stack and replaces the badges with sleek inline cards that show a spinner while loading and expand to reveal details.

**Constraint**: No changes to `internal/llm/` (LLM library swap in progress).

---

## 1. Backend: Structured Tool Metadata

### 1a. New `ToolResultMeta` type in `internal/agent/service.go`

```go
type ToolResultMeta struct {
    DurationMs    int64         `json:"durationMs"`
    ResultCount   *int          `json:"resultCount,omitempty"`
    ChunkCount    *int          `json:"chunkCount,omitempty"`
    MatchedDocs   []ToolDocRef  `json:"matchedDocs,omitempty"`
    DocumentPath  *string       `json:"documentPath,omitempty"`
    DocumentTitle *string       `json:"documentTitle,omitempty"`
    ContentLength *int          `json:"contentLength,omitempty"`
    WebResultCount *int         `json:"webResultCount,omitempty"`
    WebSources    []ToolWebRef  `json:"webSources,omitempty"`
}

type ToolDocRef struct {
    Title string  `json:"title"`
    Path  string  `json:"path"`
    Score float64 `json:"score"`
}

type ToolWebRef struct {
    Title string `json:"title"`
    URL   string `json:"url"`
}
```

### 1b. Add `Meta` field to `StreamEvent`

```go
type StreamEvent struct {
    Type    string          `json:"type"`
    Content string          `json:"content"`
    Tool    string          `json:"tool,omitempty"`
    Meta    *ToolResultMeta `json:"meta,omitempty"` // NEW: structured metadata for tool_result
}
```

### 1c. Enrich `executeTool` in `internal/agent/service.go`

For each tool case, wrap with `time.Now()` / `time.Since(start)` and build `ToolResultMeta`:

- **kb_search**: `ResultCount`, `ChunkCount` (total matched chunks), `MatchedDocs` with title/path/score, `DurationMs`
- **read_document**: `DocumentPath`, `DocumentTitle`, `ContentLength` (len of ContentBody), `DurationMs`
- **web_search**: `WebResultCount`, `WebSources` with title/url, `DurationMs`

Marshal meta to JSON, emit as `StreamEvent.Meta` field. Keep `Content` as a human-readable summary for backward compat.

### 1d. Tavily: return structured results

Modify `internal/agent/tavily.go` — change `Search` to return `(string, []tavilyResult, error)` so `executeTool` can extract title/url for `WebSources`. The `tavilyResult` struct already exists in the file.

---

## 2. DB: Store Tool Metadata

### 2a. Schema — `internal/db/schema.go`

Add after `tool_input` field:
```sql
DEFINE FIELD IF NOT EXISTS tool_meta ON message TYPE option<string>;
```

### 2b. Model — `internal/models/conversation.go`

Add field to `Message` struct:
```go
ToolMeta *string `json:"tool_meta,omitempty"`
```

### 2c. DB query — `internal/db/queries_conversation.go`

Add `toolMeta *string` param to `CreateMessage`. Add `tool_meta = $tool_meta` to the CREATE SQL and pass it in the query map.

### 2d. Agent service — `internal/agent/service.go`

When storing `RoleToolResult` messages, marshal `ToolResultMeta` to JSON string and pass as `toolMeta`.

---

## 3. GraphQL Schema

### 3a. New types in `internal/graph/schema.graphqls`

```graphql
type ToolResultMeta {
    durationMs: Int!
    resultCount: Int
    chunkCount: Int
    matchedDocs: [ToolDocRef!]
    documentPath: String
    documentTitle: String
    contentLength: Int
    webResultCount: Int
    webSources: [ToolWebRef!]
}

type ToolDocRef {
    title: String!
    path: String!
    score: Float!
}

type ToolWebRef {
    title: String!
    url: String!
}
```

Add `toolMeta: ToolResultMeta` to `ChatMessage` type.

### 3b. Code generation

Run `just generate`. Add model mappings to `gqlgen.yml` if needed.

### 3c. Helper — `internal/graph/helpers.go`

Add `toolResultMetaFromJSON(s *string) *ToolResultMeta` that unmarshals the JSON string. Call from `messageToGraphQL`; the helper handles nil internally.

---

## 4. Frontend: State Management

### 4a. Types — `web/components/domain/agent-chat-context.tsx`

```typescript
export type ToolResultMeta = {
    durationMs: number;
    resultCount?: number;
    chunkCount?: number;
    matchedDocs?: { title: string; path: string; score: number }[];
    documentPath?: string;
    documentTitle?: string;
    contentLength?: number;
    webResultCount?: number;
    webSources?: { title: string; url: string }[];
};
```

Update `StreamEvent` type to include `meta?: ToolResultMeta`.

Update `ChatMessage` type to include `toolMeta?: ToolResultMeta`.

### 4b. Replace `toolEvents` with `toolExecutions`

```typescript
type ToolExecution = {
    tool: string;
    callContent: string;
    result?: { content: string; meta?: ToolResultMeta };
};
```

State: `toolExecutions: ToolExecution[]` replaces `toolEvents: ToolEvent[]`.

Reducer:
- `STREAM_TOOL_CALL`: push new `ToolExecution` (no result yet)
- `STREAM_TOOL_RESULT`: find last execution with matching tool + no result, set its result
- `STREAM_START`: clear `toolExecutions`
- `STREAM_END`: no change

SSE parser: split `tool_call` and `tool_result` into two distinct dispatch actions.

### 4c. GraphQL queries

All conversation/message queries add `toolMeta { durationMs resultCount chunkCount matchedDocs { title path score } documentPath documentTitle contentLength webResultCount webSources { title url } }`.

---

## 5. Frontend: ToolCard Component

### New file: `web/components/domain/tool-card.tsx`

Replaces both `ToolIndicator` and `ToolMessage`.

**Props:**
```typescript
type ToolCardProps = {
    tool: string;
    callContent: string;
    result?: { content: string; meta?: ToolResultMeta };
};
```

**Visual states:**

1. **Inflight** (no result): Spinner icon + tool label + query/path text. Subtle pulse bg.
2. **Completed** (has result): Static icon + tool label + metadata badges + expand chevron.
3. **Expanded**: Matched doc list (links to `/docs{path}`) or web source links.

**Design:**
- Container: `rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2`
- Spinner: reuse existing SVG spinner pattern from `button.tsx` at `size-3.5`
- Badges: reuse `Badge` component (`variant="subtle"`, `size="sm"`) for metadata chips
- Expand animation: CSS `grid-template-rows: 0fr → 1fr` with `transition-all duration-200 motion-safe:`
- Matched docs: small text links with truncation

**Tool-specific rendering:**
- **kb_search**: Show "{N} docs" + "{M} chunks" + "{X}ms" badges. Expand shows doc titles with scores.
- **read_document**: Show document title + "{X}KB" + "{X}ms" badges. No expand needed.
- **web_search**: Show "{N} results" + "{X}ms" badges. Expand shows source links.

---

## 6. Frontend: Panel Integration

### `web/components/domain/agent-chat-panel.tsx`

**Persisted messages** — pair tool_call + tool_result into ToolCard:
```typescript
function pairMessages(messages: ChatMessage[]) {
    // Iterate messages, when tool_call found, look ahead for matching tool_result
    // Combine into { type: "tool_card", tool, callContent, result } entries
    // Non-tool messages pass through as-is
}
```

**Streaming** — render from `state.toolExecutions`:
```tsx
{chat.toolExecutions.map((exec, i) => (
    <ToolCard key={i} tool={exec.tool} callContent={exec.callContent} result={exec.result} />
))}
```

Remove old `ToolIndicator` and `ToolMessage` components.

---

## 7. i18n

### `web/messages/en.json`
```json
"agentToolKbSearch": "Knowledge Search",
"agentToolReadDoc": "Document Read",
"agentToolWebSearch": "Web Search",
"agentToolDocs": "{count} docs",
"agentToolChunks": "{count} chunks",
"agentToolDuration": "{ms}ms",
"agentToolSize": "{size}KB",
"agentToolWebResults": "{count} results",
"agentToolNotFound": "Not found"
```

### `web/messages/de.json`
Equivalent German translations.

---

## 8. Files to Modify

| File | Change |
|------|--------|
| `internal/agent/service.go` | Add `ToolResultMeta` types, enrich `executeTool`, add `Meta` to `StreamEvent` |
| `internal/agent/tavily.go` | Return `[]tavilyResult` alongside formatted string |
| `internal/models/conversation.go` | Add `ToolMeta *string` field |
| `internal/db/schema.go` | Add `tool_meta` field definition |
| `internal/db/queries_conversation.go` | Add `toolMeta` param to `CreateMessage` |
| `internal/graph/schema.graphqls` | Add `ToolResultMeta`, `ToolDocRef`, `ToolWebRef` types; add `toolMeta` to `ChatMessage` |
| `internal/graph/helpers.go` | Add `toolResultMetaFromJSON`, update `messageToGraphQL` |
| `internal/graph/schema.resolvers.go` | Auto-generated changes from `just generate` |
| `web/components/domain/agent-chat-context.tsx` | New types, `toolExecutions` state, SSE meta parsing |
| `web/components/domain/tool-card.tsx` | **NEW** — ToolCard component with spinner/badges/expand |
| `web/components/domain/agent-chat-panel.tsx` | Message pairing, replace ToolIndicator/ToolMessage with ToolCard |
| `web/messages/en.json` | New tool-related i18n keys |
| `web/messages/de.json` | German equivalents |

---

## 9. Implementation Order

1. Backend model + DB schema (`models/`, `db/schema.go`, `db/queries_conversation.go`)
2. Backend SSE enrichment (`agent/service.go` — types + executeTool + StreamEvent.Meta)
3. Tavily refactor (`agent/tavily.go` — return structured results)
4. GraphQL schema + codegen (`schema.graphqls`, `just generate`, `helpers.go`)
5. Frontend types + state (`agent-chat-context.tsx`)
6. ToolCard component (`tool-card.tsx`)
7. Panel integration (`agent-chat-panel.tsx`)
8. i18n (`en.json`, `de.json`)

---

## 10. Verification

1. `just build-all` — ensure Go compiles
2. `just test` — Go tests pass
3. `just web-lint` — TypeScript + ESLint clean
4. `just web-test` — unit tests pass
5. Manual: `just dev-all`, open chat, send a query, verify:
   - Spinner appears on tool_call SSE event
   - Spinner replaced by completed card with badges on tool_result
   - Click expand to see matched documents
   - Reload page → persisted messages render as completed ToolCards
