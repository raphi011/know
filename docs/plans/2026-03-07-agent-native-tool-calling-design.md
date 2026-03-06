# Agent Rewrite: Native Tool Calling

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
