# Agent Rewrite: Native Tool Calling — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite the agent chat to use native LLM tool calling with interleaved text+tool streaming, replacing the fragile JSON-in-prompt intent detection.

**Architecture:** Replace the two-phase intent-detect-then-answer loop with a single `GenerateStreamWithTools` call on `llm.Model` that uses eino's `model.WithTools()`. Backend emits a new SSE protocol (`text`/`tool_start`/`tool_end`/`msg_start`/`msg_end`). Frontend uses a segment-based state machine instead of separate `streamingContent` + `toolExecutions` arrays.

**Tech Stack:** Go (eino framework, `schema.ToolInfo`/`schema.ToolCall`), Next.js 16 (TypeScript, React `useReducer`), SurrealDB, SSE streaming.

**Date:** 2026-03-07
**Status:** Approved

## Problem

The agent chat has several bugs and architectural limitations:

1. **Duplicate user message** -- `messages` loaded from DB includes the just-stored current message, which then gets passed again as `currentQuery` to the LLM
2. **Misleading context label** -- web search results labeled "Context from knowledge base:" confusing the LLM
3. **Historical tool results lost** -- `buildHistory` skips all `tool_call`/`tool_result` messages, so prior turns' search results disappear
4. **No native tool calling** -- uses JSON-in-prompt intent detection instead of the LLM's built-in tool calling, causing fragile parsing and double intent prompts
5. **Flat text history for intent detection** -- `GenerateWithSystem` flattens multi-turn into a single string, losing message boundaries
6. **Two-phase flow** -- intent detection is blocking (no streaming), then answer is streamed separately. No interleaving possible.

## Design

Full rewrite of the agent backend and frontend. Migrate from JSON-in-prompt intent detection to eino's native `model.WithTools()` API. Redesign SSE protocol for interleaved text + tool calls.

### 1. LLM Layer: `GenerateStreamWithTools`

New method on `llm.Model`:

```go
func (m *Model) GenerateStreamWithTools(
    ctx context.Context,
    messages []*schema.Message,
    tools []*schema.ToolInfo,
    onToken func(token string) error,
    onToolCall func(call schema.ToolCall) (string, error),
) error
```

- Uses `model.WithTools(tools)` to pass tool definitions to eino
- Streams response, invoking `onToken` for text chunks and `onToolCall` when a tool call completes
- After `onToolCall` returns the result, appends assistant message (with tool call) + tool result message, re-calls `Stream()` with updated messages
- Loop continues until LLM generates only text (no more tool calls)

**Deleted:**
- `GenerateWithSystem` (only used for intent detection)
- `buildHistoryText` (flattened history hack)

### 2. Backend Agent Loop

**Current flow:** Load messages -> intent detection loop (up to 5 iterations) -> execute tool -> append to `toolContext` string -> stream final answer separately.

**New flow:** Build message history -> single streaming call with tools -> LLM interleaves text and tool calls natively.

```go
func (s *Service) Chat(ctx context.Context, req ChatRequest, emit func(StreamEvent)) error {
    // 1. Create/validate conversation, store user message
    // 2. Build []*schema.Message from DB (direct mapping, no hacks)
    // 3. Define []*schema.ToolInfo (kb_search, read_document, web_search if configured)
    // 4. Call model.GenerateStreamWithTools:
    //    - onToken: emit SSE "text" event + accumulate answer
    //    - onToolCall: execute tool, emit SSE "tool_start"/"tool_end", return result
    // 5. Store assistant message on completion
}
```

**Deleted:**
- `toolAction`, `searchInput`, `readDocInput`, `webSearchInput` structs
- `buildHistory`, `buildHistoryText`, `detectIntent`, `buildIntentPrompt`
- `maxToolIterations` constant
- The entire `for i := range maxToolIterations` loop
- `buildSystemPromptBase` tool documentation section (tools defined structurally)

System prompt simplifies to role/behavior only, no tool JSON format instructions.

### 3. SSE Protocol

```
Event types:
  "text"        -- streamed text token (replaces "token")
  "tool_start"  -- LLM initiated a tool call (replaces "tool_call")
  "tool_end"    -- tool execution completed (replaces "tool_result")
  "msg_start"   -- beginning of assistant message (carries message ID)
  "msg_end"     -- end of assistant message
  "conv_id"     -- conversation ID (replaces "conversation_id")
  "error"       -- error message
```

Example stream:

```
data: {"type":"conv_id","convId":"abc123"}
data: {"type":"msg_start","msgId":"msg1"}
data: {"type":"text","content":"Let me search for that."}
data: {"type":"tool_start","callId":"call_1","tool":"kb_search","input":{"query":"kubernetes"}}
data: {"type":"tool_end","callId":"call_1","tool":"kb_search","meta":{...}}
data: {"type":"text","content":"Based on your knowledge base, Kubernetes is..."}
data: {"type":"msg_end"}
```

Key changes:
- `callId` links `tool_start`/`tool_end` pairs (not matched by tool name)
- `msg_start`/`msg_end` provide clear message boundaries
- Text and tool events interleave freely
- No `"done"` event -- `msg_end` serves that purpose
- Metadata-only in `tool_end` (no full tool content sent to client)

### 4. Frontend State Machine

```typescript
type StreamSegment =
  | { type: "text"; content: string }
  | { type: "tool"; callId: string; tool: string; input: Record<string, unknown>;
      result?: { meta?: ToolResultMeta } };

type State = {
  conversations: Conversation[];
  activeConversationId: string | null;
  isStreaming: boolean;
  streamSegments: StreamSegment[];  // replaces streamingContent + toolExecutions
  error: string | null;
};
```

SSE event handling:
- `msg_start` -> `isStreaming: true`, clear `streamSegments`
- `text` -> append to last text segment (or create new one)
- `tool_start` -> push new tool segment
- `tool_end` -> find segment by `callId`, attach result
- `msg_end` -> reload conversation from server, clear `streamSegments`, set `isStreaming: false`
- `error` -> set error, stop streaming

**Deleted:**
- `streamingContent` and `toolExecutions` separate state fields
- `STREAM_TOKEN`, `STREAM_TOOL_CALL`, `STREAM_TOOL_RESULT`, `CLEAR_STREAMING` actions
- `pairMessages()` function
- Tool name-based matching

### 5. Message Storage & History

Roles:
- `user` -- user messages (unchanged)
- `assistant` -- includes serialized `tool_calls` field when LLM called tools
- `tool_result` -- tool results with `tool_call_id` linking to specific call

DB changes:
- Remove `tool_call` role (tool calls are part of assistant message)
- Add `tool_calls` JSON field on messages (stores serialized tool calls for assistant messages)
- Add `tool_call_id` field on messages (for tool_result, links to specific call)
- Keep existing `toolName`, `toolMeta` on tool_result messages

History building becomes a direct mapping:
```go
func buildMessages(dbMessages []models.Message) []*schema.Message {
    // user -> schema.User, assistant -> schema.Assistant (with ToolCalls),
    // tool_result -> schema.Tool (with ToolCallID)
}
```

GraphQL `Message` type gets `toolCalls` field and `toolCallId` field.

### 6. Testing

**Backend:**
- Unit test `GenerateStreamWithTools` with mocked eino model
- Unit test `Chat` agent loop: text-only, single tool, multiple tools, tool error, web search
- Unit test `buildMessages` DB-to-eino mapping

**Frontend:**
- Unit test reducer with mock SSE event sequences
- Update ToolCard storybook with `callId` prop
- Playwright E2E: send message, see tool card, see answer

## Files Changed

| Layer | Files | Change |
|-------|-------|--------|
| LLM | `internal/llm/model.go` | New `GenerateStreamWithTools` method |
| Agent | `internal/agent/service.go` | Full rewrite of `Chat()`, delete intent detection |
| DB/Models | `internal/models/`, `internal/db/` | Add `tool_calls`, `tool_call_id` fields |
| GraphQL | `schema.graphqls`, resolvers, helpers | Update `Message` type |
| SSE route | `web/app/api/agent/chat/route.ts` | Update event parsing |
| Frontend state | `agent-chat-context.tsx` | Rewrite reducer + SSE handler |
| Frontend UI | `agent-chat-panel.tsx`, `tool-card.tsx` | Segment-based rendering |

---

## Implementation Plan

### Task 1: DB Schema — Add `tool_calls` and `tool_call_id` fields to message table

**Files:**
- Modify: `internal/db/schema.go:234-245`
- Modify: `internal/models/conversation.go:10-40`
- Modify: `internal/db/queries_conversation.go:83-114`

**Step 1: Add new fields to SurrealDB DDL**

In `internal/db/schema.go`, add two new fields to the message table definition (after `tool_meta` line ~242):

```sql
DEFINE FIELD IF NOT EXISTS tool_call_id ON message TYPE option<string>;
DEFINE FIELD IF NOT EXISTS tool_calls   ON message TYPE option<string>;
```

**Step 2: Update Go model struct**

In `internal/models/conversation.go`, remove `RoleToolCall` constant and add new fields to `Message`:

```go
const (
    RoleUser       MessageRole = "user"
    RoleAssistant  MessageRole = "assistant"
    RoleToolResult MessageRole = "tool_result"
)

type Message struct {
    ID           surrealmodels.RecordID `json:"id"`
    Conversation surrealmodels.RecordID `json:"conversation"`
    Role         MessageRole            `json:"role"`
    Content      string                 `json:"content"`
    DocRefs      []string               `json:"doc_refs"`
    ToolName     *string                `json:"tool_name,omitempty"`
    ToolInput    *string                `json:"tool_input,omitempty"`
    ToolMeta     *string                `json:"tool_meta,omitempty"`
    ToolCallID   *string                `json:"tool_call_id,omitempty"`
    ToolCalls    *string                `json:"tool_calls,omitempty"`
    CreatedAt    time.Time              `json:"created_at"`
}
```

**Step 3: Update `CreateMessage` to accept new fields**

In `internal/db/queries_conversation.go`, add `toolCallID, toolCalls *string` params and update SQL to include them.

**Step 4: Fix all callers of `CreateMessage`**

Add `nil, nil` for the two new parameters at each existing call site (temporary — agent service rewritten in Task 3).

**Step 5: Build and test**

Run: `just build-all && just test`
Expected: PASS

**Step 6: Commit**

```
git commit -m "feat(db): add tool_calls and tool_call_id fields to message table"
```

---

### Task 2: LLM Layer — Add `GenerateStreamWithTools` method

**Files:**
- Modify: `internal/llm/model.go`
- Create: `internal/llm/tools_test.go`

**Step 1: Write failing tests**

Create `internal/llm/tools_test.go` with two tests:
- `TestGenerateStreamWithTools_TextOnly` — mock returns text chunks, verify `onToken` called for each
- `TestGenerateStreamWithTools_WithToolCall` — mock returns text + tool call on first Stream, then text on second Stream. Verify `onToken` and `onToolCall` invoked correctly, and `Stream` called twice.

Mock `BaseChatModel` using a struct that implements `Generate`/`Stream`/`BindTools`, returning `mockStreamReader` chunks.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestGenerateStreamWithTools -v`
Expected: FAIL — method doesn't exist

**Step 3: Implement `GenerateStreamWithTools`**

```go
func (m *Model) GenerateStreamWithTools(
    ctx context.Context,
    messages []*schema.Message,
    tools []*schema.ToolInfo,
    onToken func(token string) error,
    onToolCall func(call schema.ToolCall) (string, error),
) error
```

Logic:
1. Build options: `model.WithMaxTokens(8192)`, and `model.WithTools(tools)` if tools non-empty
2. Copy messages into working slice
3. Loop (max 10 iterations):
   a. Call `m.chatModel.Stream(ctx, msgs, opts...)`
   b. Collect text chunks (invoke `onToken`) and `ToolCalls` from streamed messages
   c. On EOF: if no tool calls, return nil (done)
   d. If tool calls: append `schema.AssistantMessage(text, toolCalls)` to msgs, then for each call invoke `onToolCall`, append `schema.ToolMessage(result, callID)`, continue loop
4. If loop exhausted, return error

**Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/ -run TestGenerateStreamWithTools -v`
Expected: PASS

**Step 5: Build all**

Run: `just build-all && just test`
Expected: PASS

**Step 6: Commit**

```
git commit -m "feat(llm): add GenerateStreamWithTools for native tool calling"
```

---

### Task 3: Backend — Rewrite SSE events and agent `Chat()`

**Files:**
- Modify: `internal/agent/service.go` (major rewrite)

**Step 1: Update `StreamEvent` struct**

Replace with new protocol fields: `Type`, `Content`, `ConvID`, `MsgID`, `CallID`, `Tool`, `Input`, `Meta`. All optional via `omitempty`.

**Step 2: Delete old intent detection code**

Remove: `maxToolIterations`, `toolAction`, `searchInput`, `readDocInput`, `webSearchInput`, `buildSystemPromptBase`, `buildIntentPrompt`, `buildHistory`, `buildHistoryText`, `detectIntent`, `intPtr`.

**Step 3: Add `buildMessages` helper**

Convert `[]models.Message` to `[]*schema.Message`:
- `user` -> `schema.User`
- `assistant` -> `schema.Assistant` (deserialize `ToolCalls` from JSON if present)
- `tool_result` -> `schema.ToolMessage(content, toolCallID)`

**Step 4: Add `buildTools` helper**

Return `[]*schema.ToolInfo` for `kb_search`, `read_document`, and optionally `web_search` (when `s.tavily != nil`). Use `schema.NewParamsOneOfByParams` for parameter definitions.

**Step 5: Simplify system prompt**

Replace `buildSystemPromptBase` + folder tree logic with a single `buildSystemPrompt` method. Remove tool JSON format instructions — tools are defined structurally. Keep behavioral guidelines (cite sources, ask before web search, no sources section).

**Step 6: Rewrite `Chat()` method**

New flow:
1. Create conversation if needed, emit `conv_id`
2. Store user message
3. Load history, exclude current user message (last item)
4. Build `[]*schema.Message`: system + history + doc refs context + current user
5. Emit `msg_start`
6. Call `s.model.GenerateStreamWithTools` with tools, `onToken` emitting `text`, `onToolCall` emitting `tool_start`/`tool_end` and executing via `executeTool`
7. Store assistant message (with `tool_calls` JSON) and tool result messages
8. Emit `msg_end`
9. Auto-title if first message

**Step 7: Rewrite `executeTool`**

Change signature to `(ctx, vaultID, toolName, arguments string) (string, *ToolResultMeta, error)`. Parse arguments via `json.Unmarshal` into anonymous structs. Same logic as before, just cleaner input handling.

**Step 8: Build and test**

Run: `just build-all && just test`
Expected: PASS

**Step 9: Commit**

```
git commit -m "feat(agent): rewrite Chat() with native tool calling and new SSE protocol"
```

---

### Task 4: GraphQL Schema — Update `ChatMessage` type

**Files:**
- Modify: `internal/graph/schema.graphqls`
- Modify: `internal/graph/helpers.go`
- Regenerate: `just generate`

**Step 1: Update GraphQL schema**

Add to `ChatMessage` type: `toolCallId: String`, `toolCalls: [ToolCallInfo!]`.
Add new type: `type ToolCallInfo { id: String!, name: String!, arguments: String! }`.

**Step 2: Regenerate**

Run: `just generate`

**Step 3: Update `messageToGraphQL` helper**

Add `ToolCallID` mapping and parse `ToolCalls` JSON into `[]*ToolCallInfo`.

**Step 4: Build and test**

Run: `just build-all && just test`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(graphql): add toolCallId and toolCalls to ChatMessage"
```

---

### Task 5: Frontend — Rewrite SSE types and reducer

**Files:**
- Modify: `web/components/domain/agent-chat-context.tsx`

**Step 1: Update types**

- Replace `StreamEvent` with discriminated union matching new SSE protocol
- Add `StreamSegment` type (text | tool)
- Replace `State.streamingContent` + `State.toolExecutions` with `State.streamSegments`
- Update `ChatMessage` type: add `toolCallId`, `toolCalls`, remove `tool_call` from role union
- Update `MESSAGE_FIELDS` GraphQL constant to include new fields

**Step 2: Rewrite reducer actions and cases**

Replace `STREAM_TOKEN`/`STREAM_TOOL_CALL`/`STREAM_TOOL_RESULT`/`STREAM_START`/`STREAM_END`/`CLEAR_STREAMING` with `MSG_START`/`STREAM_TEXT`/`TOOL_START`/`TOOL_END`/`MSG_END`.

Key reducer logic:
- `STREAM_TEXT`: append to last text segment or create new one
- `TOOL_START`: push new tool segment
- `TOOL_END`: find by `callId`, attach result
- `MSG_END`: clear segments, set `isStreaming: false`

**Step 3: Rewrite SSE handler in `sendMessage`**

Parse new event types, dispatch corresponding actions.

**Step 4: Verify types compile**

Run: `just web-lint`
Expected: Errors in `agent-chat-panel.tsx` (fixed in Task 6)

**Step 5: Commit**

```
git commit -m "feat(web): rewrite agent chat state machine for new SSE protocol"
```

---

### Task 6: Frontend — Update panel and tool card rendering

**Files:**
- Modify: `web/components/domain/agent-chat-panel.tsx`
- Modify: `web/components/domain/tool-card.tsx`

**Step 1: Update `ToolCard` props**

Add optional `callId` prop.

**Step 2: Rewrite persisted message rendering**

Replace `pairMessages()` with `buildEntries()` that:
- Indexes `tool_result` messages by `toolCallId`
- For `assistant` messages with `toolCalls`, renders a `ToolCard` per call (looking up results by call ID), then the text content
- Skips standalone `tool_result` messages (already rendered via assistant's `toolCalls`)

**Step 3: Rewrite streaming rendering**

Iterate `chat.streamSegments` — render `ToolCard` for tool segments, `MarkdownRenderer` for text segments.

**Step 4: Update typing indicator and auto-scroll**

Depend on `chat.streamSegments` instead of `chat.streamingContent`/`chat.toolExecutions`.

**Step 5: Verify**

Run: `just web-lint`
Expected: PASS

**Step 6: Commit**

```
git commit -m "feat(web): update chat panel for segment-based rendering"
```

---

### Task 7: Frontend — Extract and test reducer

**Files:**
- Create: `web/components/domain/agent-chat-reducer.ts`
- Create: `web/components/domain/agent-chat-reducer.test.ts`
- Modify: `web/components/domain/agent-chat-context.tsx` (import from new file)

**Step 1: Extract reducer**

Move `reducer`, `State`, `Action`, `StreamSegment`, `initialState` to `agent-chat-reducer.ts`. Re-export from `agent-chat-context.tsx`.

**Step 2: Write tests**

Test cases:
- `MSG_START` clears segments, sets streaming
- `STREAM_TEXT` appends to existing text segment
- `STREAM_TEXT` after tool creates new text segment
- `TOOL_START` creates tool segment
- `TOOL_END` attaches result by callId
- `MSG_END` clears state
- Full interleaved sequence: text -> tool_start -> tool_end -> text = 3 segments

**Step 3: Run tests**

Run: `cd web && bun test agent-chat-reducer`
Expected: PASS

**Step 4: Commit**

```
git commit -m "test(web): add unit tests for agent chat reducer"
```

---

### Task 8: Integration — End-to-end verification

**Step 1: Run all backend tests**

Run: `just test`
Expected: PASS

**Step 2: Run all frontend tests and lint**

Run: `just web-lint && just web-test`
Expected: PASS

**Step 3: Full build**

Run: `just build-all && just web-build`
Expected: PASS

**Step 4: Manual smoke test**

Run: `just dev-all`

Test:
1. kb_search flow — tool card appears, answer follows
2. Multi-turn — prior tool results preserved in history
3. Web search gating — agent asks permission before web search
4. Web search — tool card with sources, answer uses results
5. New conversation — clean state

**Step 5: Final commit if needed**

```
git commit -m "feat: complete agent rewrite with native tool calling"
```
