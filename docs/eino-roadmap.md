# Eino Full Migration Roadmap

Know uses CloudWeGo's Eino framework (v0.8.1) but only leverages low-level primitives. The entire agent loop, tool dispatch, observability, state management, and approval workflow are hand-rolled. Eino provides production-ready solutions for all of these.

**Goal**: Replace all custom agent/orchestration code with eino's built-in capabilities end-to-end.

**No backwards compatibility concerns**: Per project policy, breaking changes to APIs, DB schema, SSE events, etc. are all fine.

**Reference**: See `~/Git/second-brain/02 Notes/Programming/Eino Framework.md` for full eino API documentation.

---

## Current State

### What we use from eino

| Package | Types / Functions |
|---------|-------------------|
| `schema` | `Message`, `ToolCall`, `ToolInfo`, `ParameterInfo`, `NewParamsOneOfByParams`, `ConcatMessages`, `StreamReader`, `MessageInputPart`, `RoleType` constants |
| `components/model` | `BaseChatModel` (Generate, Stream), `model.WithMaxTokens` |
| `components/embedding` | `Embedder` (EmbedStrings) |
| `eino-ext` | `model/claude`, `model/openai`, `model/gemini`, `model/ollama`, `embedding/*` |
| `adk` | `ChatModelAgent`, `Runner`, `RunnerConfig.CheckPointStore`, `ResumeWithParams`, `WithCheckPointID`, `SendEvent`, `AddSessionValue`/`AddSessionValues`/`GetSessionValue`/`GetSessionValues`, `NewAsyncIteratorPair`, middleware interfaces (`BeforeAgent`, `BeforeModelRewriteState`, `AfterModelRewriteState`, `WrapInvokableToolCall`) |
| `compose` | `ToolsNodeConfig` (via ADK), `IsInterruptRerunError` |
| `tool` | `StatefulInterrupt`, `GetInterruptState`, `GetResumeContext` |

### What we do NOT use (yet)

| eino Feature | Status |
|-------------|--------|
| ADK (`ChatModelAgent`, `Runner`) | ✅ Used — ReAct agent loop (Phase 3) |
| `compose.*` (Graph, Chain, ToolsNode) | ✅ Used — ADK uses ToolsNode internally (Phase 3) |
| `callbacks.Handler` | ✅ Used — global observability handler (Phase 2) |
| `adk.SendEvent` / custom events | ✅ Used — tool/approval events via middleware (Phase 3) |
| `adk.AddSessionValue` / `GetSessionValue` | ✅ Used — token tracking + tool records (Phase 3), FString prompt templating (Phase 5) |
| Interrupt/Resume, CheckPointStore | ✅ Used — `approvalToolWrapper` + SurrealDB checkpoint store (Phase 4) |
| `tool.InvokableTool` | ✅ Used — registry-based dispatch (Phase 1) |
| `retriever.Retriever`, `indexer.Indexer` | Not used — direct DB queries with `search::rrf()` (Phase 6) |

---

## What Our Custom Code Does (to be replaced)

| Custom Code | File | Lines | Eino Replacement | Status |
|-------------|------|-------|-----------------|--------|
| Agent loop (Chat method) | `agent/service.go` | ~290 | ADK `ChatModelAgent` ReAct loop | ✅ Phase 3 |
| Tool calling loop | `llm/model.go` GenerateStreamWithTools | ~110 | ADK handles internally | ✅ Phase 3 |
| Tool dispatch (sequential switch) | `tools/executor.go` ExecuteTool | ~60 | `compose.ToolsNode` (parallel) | ✅ Phase 1+3 |
| Tool definitions | `agent/service.go` buildTools | ~145 | `tool.InvokableTool` implementations | ✅ Phase 1+3 |
| System prompt building | `agent/service.go` buildSystemPrompt | ~45 | `BeforeAgent` session values + FString template | ✅ Phase 5 |
| Message history assembly | `agent/service.go` buildMessages | ~25 | `ChatModelAgentState.Messages` | ✅ Phase 3 |
| Doc ref / attachment injection | `agent/service.go` Chat lines 386-435 | ~50 | `BeforeModelRewriteState` middleware | ✅ Phase 3 |
| Approval workflow | `agent/approval.go` | 83 | `approvalToolWrapper` + `CheckPointStore` interrupt/resume | ✅ Phase 4 |
| Token tracking (manual) | `llm/model.go` extractTokenCounts | ~30 | Session values + `ResponseMeta` | ✅ Phase 3 |
| Manual LLM observability | `llm/model.go` scattered | ~40 | Callbacks system | ✅ Phase 2 |

---

## Phase 1: Tools as InvokableTool ✅

**Goal**: Convert tool definitions from `schema.ToolInfo` + manual dispatch to `tool.InvokableTool` implementations.

### What was done

- Each tool (`search`, `read_document`, `list_labels`, `list_folders`, `list_folder_contents`, `create_document`, `edit_document`, `edit_document_section`, `create_memory`, `get_document_versions`) is now a struct implementing `tool.InvokableTool` in its own file under `internal/tools/`
- `tools/executor.go` switch statement replaced by a lazy-initialized registry of `InvokableTool` implementations
- `executor.Tools()` exposes all tools as `[]tool.BaseTool` for future `ToolsNode` / ADK integration
- Agent's `buildTools()` now delegates to `executor.Tools()` instead of duplicating 145 lines of `schema.ToolInfo` definitions
- Agent tool names unified to canonical names (`kb_search` → `search`), `agentToolToCanonical` map removed
- `web_search` remains agent-only (not in executor — it's a Tavily API call, not a vault tool)
- MCP tools unchanged — they still use `ToolExecutor.ExecuteTool()` which dispatches through the registry internally

### Design decisions (as implemented)

- `ToolResultMeta` returned via context (`WithResultMeta`/`ResultMeta`/`SetResultMeta`) since `InvokableRun` returns `(string, error)`
- Vault ID passed via `tool.Option` using eino's `WrapImplSpecificOptFn` pattern (`WithVaultID`)
- The `ToolExecutor` interface (`ExecuteTool(ctx, vaultID, name, args)`) remains for the remote proxy
- ToolsNode not wired yet — that happens in Phase 3 when ADK `ChatModelAgent` manages the tool dispatch loop

### Key files

| File | Change |
|------|--------|
| `internal/tools/executor.go` | Switch dispatch → registry-based `InvokableTool` dispatch |
| `internal/tools/options.go` | `WithVaultID` tool option |
| `internal/tools/meta.go` | Context-based `ToolResultMeta` passing |
| `internal/tools/tool_*.go` (10 files) | Individual tool structs |
| `internal/agent/service.go` | `buildTools()` delegates to executor, canonical names, no more mapping |

---

## Phase 2: Callbacks System ✅

**Goal**: Replace manual observability with eino's callback handlers.

### What was done

- Global callback handler registered at startup via `callbacks.AppendGlobalHandlers`
- Uses `HandlerBuilder` with `OnStart`/`OnEnd`/`OnError` — logs component lifecycle via `logutil.FromCtx(ctx)` with structured fields (component type, duration, token usage breakdown)
- For ChatModel callbacks: extracts `model.CallbackOutput.TokenUsage` for rich logging (prompt tokens, completion tokens, cached tokens)
- Foundation for OpenTelemetry traces later

### Design decisions (as implemented)

- **Logging only in callbacks, metrics stay in wrappers**: All eino-ext model/embedding providers (Claude, OpenAI, Ollama, Gemini) fire callbacks. The custom Bedrock embedder (langchaingo-based) bypasses eino entirely. Since eino callbacks return a new context that is NOT propagated back to the caller (`chatModel.Generate()` returns the original context), there's no way to deduplicate metrics. Therefore: callbacks handle **structured logging** (additive, harmless if doubled), and `Model`/`Embedder` wrappers handle **metrics** (universal, works for all providers including Bedrock).
- **Start time via context key**: `OnStart` stores `time.Now()` as a context value; `OnEnd`/`OnError` compute duration from it.
- **Registered once at startup**: `llm.RegisterCallbacks()` called in `server.New()`. Not called on SIGHUP reload (global handlers persist across model swaps since they're process-global).

### Key files

| File | Change |
|------|--------|
| New: `internal/llm/callbacks.go` | Observability callback handler (~80 lines) |
| New: `internal/llm/callbacks_test.go` | Tests for handler registration and lifecycle |
| `internal/server/bootstrap.go` | `llm.RegisterCallbacks()` called at startup |

### Effort: S

Additive, no behavioral changes.

---

## Phase 3: ADK ChatModelAgent Migration ✅

**Goal**: Replace the custom agent loop with `ChatModelAgent`. This is the biggest change.

**Key constraint**: Keep approval channel-based (Phase 4 converts to interrupt/resume). Keep SSE event types/structure unchanged (TUI depends on them).

### What was done

- `Chat()` (~290 lines) decomposed into `buildAgent()`, `consumeAgentEvents()`, `persistResults()`
- `GenerateStreamWithTools()` (~110 lines) **deleted** — ADK handles the ReAct loop
- `buildTools()`, `executeTool()`, `execWebSearch()` **deleted** — tools passed directly to ADK config
- `llm.TokenUsage` **moved** to `agent.TokenUsage` in new `events.go`
- `web_search` extracted to `agent/websearch_tool.go` as `WebSearchTool` implementing `tool.InvokableTool`
- `llm.Model.BaseChatModel()` accessor **added** — exposes underlying model for ADK
- Custom `GenModelInput` to skip FString interpolation (system prompt contains JSON with curly braces)

### Architecture

**Middleware stack** — 3 middlewares, each embedding `*adk.BaseChatModelAgentMiddleware`:

| # | Middleware | Hook | Purpose |
|---|-----------|------|---------|
| 1 | `contextInjectionMiddleware` | `BeforeModelRewriteState` | Inject doc refs + text attachments as messages before first model call |
| 2 | `tokenTrackingMiddleware` | `AfterModelRewriteState` | Accumulate token usage in ADK session values |
| 3 | `toolExecutionMiddleware` | `WrapInvokableToolCall` | Gate write tools on approval, emit events via `adk.SendEvent`, record tool calls (mutex-protected for parallel execution) |

**Custom event types** — sent via `adk.SendEvent`, translated to SSE in `consumeAgentEvents`:

| Event | Source | Purpose |
|-------|--------|---------|
| `ToolStartEvent` | `toolExecutionMiddleware` | Tool call began |
| `ToolEndEvent` | `toolExecutionMiddleware` | Tool call completed (success or error) |
| ~~`ApprovalRequiredEvent`~~ | ~~`toolExecutionMiddleware`~~ | Removed in Phase 4 — replaced by `interrupted` SSE event from `approvalToolWrapper` |
| `RunCompleteEvent` | `sessionDumpAgent` wrapper | Carries accumulated token usage + tool records |

**`sessionDumpAgent`** — wraps the inner agent to emit `RunCompleteEvent` after all events are drained. Includes panic recovery to ensure token usage is always captured.

**SSE streaming** — `consumeAgentEvents` drains `AsyncIterator[*AgentEvent]`, consuming `MessageStream` chunks for per-token SSE. Continues draining after errors to capture `RunCompleteEvent`.

**Agent construction** — per-request because middlewares carry per-request state. `ChatModelAgent` construction is cheap (react graph compiles lazily on first `Run()`).

### Design decisions

- **Session values over struct fields**: Token usage and tool records accumulated via `adk.AddSessionValue`/`GetSessionValue` rather than middleware struct fields. This works with ADK's session system and enables `sessionDumpAgent` to extract them after the run.
- **Mutex for parallel tool safety**: `toolExecutionMiddleware.mu` protects `recordTool()`'s read-modify-write on session values since ADK's `ToolsNode` executes tools in parallel by default.
- **`consumeAgentEvents` continues after errors**: Uses `hitError` flag to stop emitting SSE but keeps draining the iterator so `RunCompleteEvent` (with token data) is always captured.
- **`buildAgent` returns error**: Propagates `NewChatModelAgent` failures cleanly instead of risking nil-deref panics.
- **`persistResults` aggregates errors**: Tool result storage failures collected via `errors.Join` and surfaced to caller.

### Key files

| File | Change |
|------|--------|
| `internal/agent/service.go` | `Chat()` → `buildAgent()` + `consumeAgentEvents()` + `persistResults()`, removed `buildTools`/`executeTool`/`execWebSearch` |
| `internal/agent/middleware.go` | 3 middleware structs + `sessionDumpAgent` wrapper (~330 lines) |
| New: `internal/agent/events.go` | `TokenUsage`, `ToolRecord`, `AgentResult`, custom event types |
| New: `internal/agent/websearch_tool.go` | `WebSearchTool` InvokableTool |
| `internal/llm/model.go` | `BaseChatModel()` added, `GenerateStreamWithTools` + `TokenUsage` deleted |

### Unchanged files

- `handler.go` — HTTP handlers unchanged (still calls `s.service.Chat(bgCtx, req, emit)`)
- `runner.go` — `Runner.Start()` still calls `s.service.Chat()`, unchanged
- `approval.go` — channel-based approval unchanged (Phase 4 converts to interrupt/resume)
- `tools/executor.go` — tool registry unchanged
- `tools/tool_*.go` — all InvokableTool implementations unchanged

### Effort: L

Biggest change — eliminated ~500 lines of custom orchestration, replaced with composable middleware.

---

## Phase 4: Interrupt/Resume for Approvals ✅

**Goal**: Replace in-memory approval channels with eino's checkpoint-based interrupt/resume.

### What was done

- `approvalRegistry` + `sync.Map` + channel-based blocking **deleted** — replaced by eino's `tool.StatefulInterrupt` / `tool.GetInterruptState` / `tool.GetResumeContext`
- New `approvalToolWrapper` wraps each write tool at the tool level (not middleware), following eino's intended pattern from `interrupt_test.go` and `integration_test.go`
- SurrealDB-backed `SurrealCheckPointStore` implements `adk.CheckPointStore` — agent state survives server restarts
- New `Runner.Resume()` method rebuilds agent and calls `adkRunner.ResumeWithParams()` with the user's approval response
- New `interrupted` SSE event type (replaces `tool_approval_required`) — includes `interruptId` for resume targeting
- Gob type registration (`schema.RegisterName`) for all types flowing through checkpoint serialization
- `toolExecutionMiddleware.WrapInvokableToolCall` simplified: emits events + records tool calls, but no longer blocks on approval channels; interrupt errors propagated via `compose.IsInterruptRerunError()`
- TUI resubscribes to SSE events after approval via `resubscribeAfterApproval()`

### Architecture

**Interrupt flow** (first call — write tool):

1. `approvalToolWrapper.InvokableRun()` detects first call (`!wasInterrupted`)
2. Computes diff via `service.buildApprovalRequest()`
3. Calls `tool.StatefulInterrupt(ctx, approvalReq, &approvalToolState{...})`
4. Interrupt error propagates up through compose graph → ADK checkpoints state to SurrealDB
5. `consumeAgentEvents` detects `event.Action.Interrupted`, emits `interrupted` SSE event with `interruptId`
6. Agent goroutine terminates (no blocking)

**Resume flow** (after user approves/rejects):

1. `POST /agent/approval` sends `{conversationId, interruptId, action}`
2. `Runner.Resume()` builds fresh agent, calls `adkRunner.ResumeWithParams(ctx, checkpointID, &ResumeParams{Targets: ...})`
3. ADK restores state from checkpoint, re-enters `approvalToolWrapper.InvokableRun()`
4. Wrapper detects resume via `tool.GetInterruptState()` + `tool.GetResumeContext()`
5. If approved: delegates to inner tool's `InvokableRun()` (with hunk-filtered args if partial approval)
6. If rejected: returns rejection message as string (not error)
7. Agent continues normal ReAct loop from the tool result

**Parallel tool handling**: When multiple write tools interrupt, only one is the `isResumeTarget`. Non-target siblings re-interrupt with saved state, preserving checkpoint consistency.

### Design decisions

- **Approval at tool level, not middleware**: Eino's interrupt/resume is designed for tools — `tool.GetInterruptState()` and `tool.GetResumeContext()` work at the `InvokableRun` boundary. All eino test code follows this pattern.
- **ConversationID as checkpoint ID**: Natural 1:1 mapping — one conversation = one checkpoint. Resume uses conversation ID to locate checkpoint.
- **`approvalToolWrapper` wraps only write tools**: Read-only tools pass through unwrapped. The wrapper delegates `Info()` to the inner tool, so tool schemas are unchanged.
- **Partial approval (hunk selection) via resume data**: `ApprovalResponse` (with `Action` and `HunkIndexes`) is the resume data. The wrapper applies hunk selection before delegating to the inner tool.
- **`sessionDumpAgent` captures partial results on interrupt**: Token usage from iterations before the interrupt is still recorded via `RunCompleteEvent`.
- **`Chat()` skips `msg_end` on interrupt**: Partial results persisted, but `msg_end` SSE event only emitted after resume completes.
- **`approval_test.go` deleted**: Tests for the deleted `approvalRegistry` — no longer relevant.

### Key files

| File | Change |
|------|--------|
| New: `internal/agent/approval_tool.go` (~149 lines) | `approvalToolWrapper` — interrupt/resume lifecycle for write tools |
| New: `internal/agent/checkpoint.go` (~32 lines) | `SurrealCheckPointStore` backed by SurrealDB |
| New: `internal/db/queries_checkpoint.go` (~41 lines) | `GetCheckpoint` / `UpsertCheckpoint` DB methods |
| `internal/agent/approval.go` | Deleted `approvalRegistry`, kept `ApprovalRequest`/`ApprovalResponse`/`ApprovalAction` types |
| `internal/agent/service.go` | Removed `activeApprovals sync.Map`, wire checkpoint store + tool wrapping, handle interrupt in `consumeAgentEvents` |
| `internal/agent/middleware.go` | Simplified `WrapInvokableToolCall` — removed channel blocking, propagate interrupt errors |
| `internal/agent/runner.go` | Added `Resume()` method + `ResumeRequest` struct |
| `internal/agent/handler.go` | `HandleApproval` moved to `Runner`, uses `interruptId` for resume |
| `internal/agent/events.go` | Gob `RegisterName` for checkpoint serialization, `Interrupted` field on `AgentResult` |
| `internal/db/schema.go` | `agent_checkpoint` table definition |
| `internal/tui/app.go` | Handle `interrupted` event, `resubscribeAfterApproval()` |
| `internal/tui/client.go` | `InterruptID` field, updated `Approve()` to send `interruptId` |
| `cmd/know/cmd_serve.go` | Route `HandleApproval` via `AgentRunner()` |

### Effort: M

~300 lines added, ~236 removed in modified files, plus ~222 lines in 3 new files.

---

## Phase 5: Session Management + GenModelInput ✅

**Goal**: Use eino's session system with DB-backed hydration for system prompt templating.

### What was done

- `buildSystemPrompt()` (inline DB queries + string concatenation) **deleted** — replaced by `instructionTemplate` const with FString `{FolderTree}` and `{Labels}` placeholders
- `contextInjectionMiddleware.BeforeAgent` hydrates session values from DB (ListFolders, ListLabelsWithCounts)
- Custom `GenModelInput` **removed** — eino's default FString-based `GenModelInput` now interpolates session values into the instruction template
- `formatFolderTree()` and `formatLabels()` helper functions produce identical output to the old `buildSystemPrompt` formatting (including section headers inside the value, so empty → no orphan headers)

### Design decisions

- **Extended `contextInjectionMiddleware`** rather than adding a 4th middleware — it already holds `vaultID` and `db`
- **Default FString GenModelInput** — prompt has no literal curly braces, so FString works cleanly. Extra session keys (`token_usage`, `tool_records`) are ignored by pyfmt since they don't appear as `{key}` in the template
- **Session values are plain strings** — no gob registration needed (string is a primitive)
- **BeforeAgent re-runs on Resume** — folder tree / labels re-fetched from DB, which is correct (data may have changed during interruption)

### Key files

| File | Change |
|------|--------|
| `internal/agent/middleware.go` | Added `BeforeAgent` to `contextInjectionMiddleware`, added `formatFolderTree` + `formatLabels` helpers |
| `internal/agent/service.go` | Added `instructionTemplate` const, deleted `buildSystemPrompt`, removed custom `GenModelInput`, updated `buildAgent` signature + call sites |
| `internal/agent/runner.go` | Updated `ResumeChat` call site (removed `buildSystemPrompt` + updated `buildAgent` args) |

### Effort: S

~45 lines removed, ~65 lines added.

---

## Phase 6: SurrealDB Hybrid Search (`search::rrf`) ✅

**Goal**: Replace Go-side RRF fusion with SurrealDB's built-in `search::rrf()`, reducing three DB round-trips + Go fusion to a single query.

### What was done

- `Search()` simplified from ~85 lines to ~45 lines — BM25 query, vector query, and RRF fusion collapse into one `search::rrf()` call
- Go-side fusion code deleted: `rrfFusion`, `collectDocIDs`, `fetchDocMap`, `buildDocMap`, `aggregateChunks`, `chunksToResults`, `chunkKey`, `docInfo`, `docInfoFromModel`
- `ChunkWithScore` extended with doc metadata fields (`DocPath`, `DocTitle`, `DocLabels`, `DocType`) via record link traversal — eliminates separate `GetDocumentsByIDs` call from search
- `BM25ChunkSearch` updated to include doc fields (same record link pattern)
- `ChunkVectorSearch` deleted — replaced by hybrid query
- `embedQuery()` extracted as a clean method (was inline in `Search()`), takes `llm.Embedder` as parameter to avoid TOCTOU race on atomic pointer
- New `assembleResults()` unifies both BM25-only and hybrid paths
- Doc score uses **sum** of chunk scores (preserves hybrid boost for documents appearing in both BM25 and vector)
- All logging uses `logutil.FromCtx(ctx)` for request-scoped context propagation
- Double-failure error chains include both the original error and the fallback error

### Architecture

**Before**:
```
Go: BM25Query → DB
Go: EmbedQuery → Embedder API
Go: VectorQuery → DB
Go: rrfFusion(bm25, vector) → Go code
Go: fetchDocMap(docIDs) → DB
Go: assembleResults → Go code
```

**After**:
```
Go: EmbedQuery → Embedder API (with cache)
Go: HybridSearch(query, embedding) → DB (single search::rrf query with doc metadata)
Go: assembleResults → Go code
```

### SurrealDB `search::rrf()` query

```sql
SELECT *,
    document.path AS doc_path,
    document.title AS doc_title,
    document.labels AS doc_labels,
    document.doc_type AS doc_type
FROM search::rrf([
    (SELECT * FROM chunk WHERE <filters> AND content @1@ $query LIMIT $limit),
    (SELECT * FROM chunk WHERE <filters> AND embedding <|$limit,40|> $embedding LIMIT $limit)
], $limit, 60)
```

**Gotchas discovered**:
- `LET` variables return `None` when passed to `search::rrf()` — subqueries must be inlined
- `search::score()` cannot be used inside parenthesized subqueries within `search::rrf()` (parse error on `::`) — omit `ORDER BY` since BM25 `@1@` and KNN `<|K,EF|>` return results in relevance order implicitly
- Record link traversal (`document.title AS doc_title`) works in the outer `SELECT`

### Graceful degradation (unchanged behavior)

- **No embedder / BM25Only**: `BM25ChunkSearch` directly
- **Embedding fails**: fall back to BM25-only, mark results as `Degraded: true`
- **HybridSearch fails**: fall back to BM25-only, mark results as `Degraded: true`

### Key files

| File | Change |
|------|--------|
| `internal/db/queries_search.go` | New `HybridSearch` with `search::rrf()`, updated `BM25ChunkSearch` with doc fields, deleted `ChunkVectorSearch` |
| `internal/search/service.go` | Deleted ~200 lines of fusion code, simplified `Search()`, extracted `embedQuery()`, new `assembleResults()` |
| `internal/search/service_test.go` | Replaced RRF/aggregate tests with `assembleResults` tests |
| `internal/db/queries_search_test.go` | Replaced `TestChunkVectorSearch` with `TestHybridSearch` |

### Effort: S

~200 lines removed, ~80 lines added.

---

## Phase 7+ (Deferred): Future Improvements

The following ideas were evaluated after Phase 6 and deferred — the current search pipeline (hybrid BM25+vector via `search::rrf()`, top-20 results fed to the agent) works well enough for a personal knowledge base. All three add latency and LLM cost with marginal benefit when the agent already reads all top results.

### HyDE (Hypothetical Document Embeddings)

LLM generates a hypothetical answer, embed *that* for vector search instead of the raw query. Improves recall when queries use different vocabulary than documents. Adds ~1-2s latency per search (one LLM call). **Revisit if**: short/vague queries consistently miss relevant documents.

### LLM Reranking

Post-fusion step that sends top-N results to the LLM to reorder by relevance. **Revisit if**: top-5 results are consistently wrong despite good recall (i.e., the right documents appear but are ranked poorly).

### compose.Chain for Search Pipeline

Wrap search steps in `compose.Chain` for per-step callbacks (timing, logging via Phase 2 callback system). Only worth it if HyDE or reranking are added (multiple steps to instrument). **Revisit if**: either of the above is adopted.

### Multi-Agent Patterns

Leverage eino's agent hierarchy (`adk/prebuilt/supervisor`, `planexecute`, `deep`) for complex workflows like multi-document research or specialist routing. **Revisit if**: single-agent performance becomes a bottleneck for complex tasks.

---

## Implementation Order

```
Phase 1: InvokableTool ✅ ───┐
                              ├─→ Phase 3: ADK ChatModelAgent ✅ ─┬─→ Phase 4: Interrupt/Resume ✅
Phase 2: Callbacks ✅ ───────┘                                    └─→ Phase 5: Session + GenModelInput ✅
                                                                       Phase 6: SurrealDB Hybrid Search ✅
```

- **Phases 1–6**: ✅ complete — full eino migration done
- **Phase 7+**: deferred — HyDE, LLM reranking, compose.Chain, multi-agent all evaluated and shelved (see above)

---

## Key Files Summary

| File | Lines | Role | Phases |
|------|-------|------|--------|
| `internal/agent/service.go` | ~630 | ADK agent construction, event consumption, persistence, interrupt handling | ✅ 3, 4, 5 |
| `internal/agent/handler.go` | ~346 | REST API endpoints (HandleApproval moved to Runner) | ✅ 3, 4 |
| `internal/agent/runner.go` | ~331 | Background execution, SSE event replay, `Resume()` for interrupt/resume | ✅ 3, 4 |
| `internal/agent/middleware.go` | ~284 | ADK middleware (context injection, token tracking, tool execution) | ✅ 3, 4, 5 |
| `internal/agent/approval_tool.go` | ~149 | `approvalToolWrapper` — interrupt/resume lifecycle for write tools | ✅ 4 |
| `internal/agent/events.go` | ~61 | Custom event types, TokenUsage, ToolRecord, AgentResult, gob registration | ✅ 3, 4 |
| `internal/agent/websearch_tool.go` | ~60 | WebSearchTool InvokableTool | ✅ 3 |
| `internal/agent/approval.go` | ~34 | Approval types (ApprovalRequest/Response/Action) | ✅ 4 |
| `internal/agent/checkpoint.go` | ~32 | SurrealDB CheckPointStore | ✅ 4 |
| `internal/llm/model.go` | ~545 | LLM wrapper, BaseChatModel() accessor | ✅ 2, 3 |
| `internal/tools/executor.go` | ~110 | Registry-based tool dispatch via InvokableTool | ✅ 1 |
| `internal/tools/tool_*.go` | ~900 | Individual InvokableTool implementations (10 tools) | ✅ 1 |
| `internal/tools/meta.go` | ~40 | Context-based ToolResultMeta passing (SetResultMeta exported) | ✅ 1, 3 |
| `internal/mcptools/tools.go` | 561 | MCP tool bridge (shares executor) | ✅ 1 (unchanged) |
| `internal/search/service.go` | ~220 | Simplified hybrid search via `search::rrf()` | ✅ 6 |
| `internal/db/queries_search.go` | ~140 | BM25 + HybridSearch queries | ✅ 6 |
| `internal/metrics/collector.go` | 203 | Metrics collection | ✅ 2 |
| `internal/db/queries_checkpoint.go` | ~41 | GetCheckpoint / UpsertCheckpoint DB methods | ✅ 4 |

---

## Lines Eliminated (Estimated)

| Phase | Lines Removed | Lines Added | Net |
|-------|--------------|-------------|-----|
| Phase 1 ✅ | 699 (executor switch + buildTools + name mapping) | 63 (modified) + ~900 (tool structs + infra) | ~-636 net in modified files |
| Phase 2 ✅ | 0 (additive — manual timing kept for providers without callbacks) | ~60 (callback handler) + ~50 (tests) | +110 |
| Phase 3 ✅ | 376 (Chat loop + GenerateStreamWithTools + buildTools + executeTool) | 515 (middleware + events + agent wiring) | +139 |
| Phase 4 ✅ | 236 (approval registry + channel blocking + approval_test.go) | 300 (modified) + 222 (3 new files) | +286 |
| Phase 5 ✅ | ~45 (buildSystemPrompt + custom GenModelInput) | ~65 (BeforeAgent + formatters + template const) | +20 |
| Phase 6 | ~200 (Go-side RRF fusion + doc fetch + aggregation) | ~40 (HybridSearch DB method) | -160 |
| **Total** | **~1556** | **~2170** | **+614** |

Net increase of ~614 lines, but the new code is composable, testable middleware with proper error handling, race protection, and crash-resilient checkpoint persistence — replacing monolithic orchestration with in-memory state.
