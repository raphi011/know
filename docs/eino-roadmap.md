# Eino Full Migration Roadmap

Knowhow uses CloudWeGo's Eino framework (v0.8.1) but only leverages low-level primitives. The entire agent loop, tool dispatch, observability, state management, and approval workflow are hand-rolled. Eino provides production-ready solutions for all of these.

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
| `adk` | `ChatModelAgent`, `Runner`, `SendEvent`, `AddSessionValue`/`GetSessionValue`, `NewAsyncIteratorPair`, middleware interfaces |
| `compose` | `ToolsNodeConfig` (via ADK) |

### What we do NOT use (yet)

| eino Feature | Status |
|-------------|--------|
| ADK (`ChatModelAgent`, `Runner`) | ✅ Used — ReAct agent loop (Phase 3) |
| `compose.*` (Graph, Chain, ToolsNode) | ✅ Used — ADK uses ToolsNode internally (Phase 3) |
| `callbacks.Handler` | ✅ Used — global observability handler (Phase 2) |
| `adk.SendEvent` / custom events | ✅ Used — tool/approval events via middleware (Phase 3) |
| `adk.AddSessionValue` / `GetSessionValue` | ✅ Used — token tracking + tool records (Phase 3) |
| Interrupt/Resume, CheckPointStore | Not used — in-memory approval channels (Phase 4) |
| `tool.InvokableTool` | ✅ Used — registry-based dispatch (Phase 1) |
| `retriever.Retriever`, `indexer.Indexer` | Not used — direct DB queries |

---

## What Our Custom Code Does (to be replaced)

| Custom Code | File | Lines | Eino Replacement | Status |
|-------------|------|-------|-----------------|--------|
| Agent loop (Chat method) | `agent/service.go` | ~290 | ADK `ChatModelAgent` ReAct loop | ✅ Phase 3 |
| Tool calling loop | `llm/model.go` GenerateStreamWithTools | ~110 | ADK handles internally | ✅ Phase 3 |
| Tool dispatch (sequential switch) | `tools/executor.go` ExecuteTool | ~60 | `compose.ToolsNode` (parallel) | ✅ Phase 1+3 |
| Tool definitions | `agent/service.go` buildTools | ~145 | `tool.InvokableTool` implementations | ✅ Phase 1+3 |
| System prompt building | `agent/service.go` buildSystemPrompt | ~45 | `GenModelInput` + session values | Phase 5 |
| Message history assembly | `agent/service.go` buildMessages | ~25 | `ChatModelAgentState.Messages` | ✅ Phase 3 |
| Doc ref / attachment injection | `agent/service.go` Chat lines 386-435 | ~50 | `BeforeModelRewriteState` middleware | ✅ Phase 3 |
| Approval workflow | `agent/approval.go` | 83 | Interrupt/Resume + `WrapInvokableToolCall` | Phase 4 |
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
| `ApprovalRequiredEvent` | `toolExecutionMiddleware` | Write tool needs user approval |
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

## Phase 4: Interrupt/Resume for Approvals

**Goal**: Replace in-memory approval channels with eino's checkpoint system.

### What changes

- `sync.Map` approval registry (`activeApprovals`) → checkpoint-based interrupt/resume
- Write tool calls trigger `adk.StatefulInterrupt(ctx, approvalInfo, toolCallState)` from `WrapInvokableToolCall`
- Approval REST endpoint calls `runner.Resume(ctx, ResumeParams{...})`
- Agent state survives server restarts (checkpoint persisted to SurrealDB)
- SurrealDB-backed `CheckPointStore` implementation

### CheckPointStore implementation

```go
type SurrealCheckPointStore struct {
    db *db.Client
}

func (s *SurrealCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
    // SELECT data FROM agent_checkpoint WHERE id = $id
}

func (s *SurrealCheckPointStore) Set(ctx context.Context, id string, data []byte) error {
    // UPSERT agent_checkpoint SET data = $data, updated_at = time::now()
}
```

### Interrupt flow

1. `ApprovalMiddleware.WrapInvokableToolCall` checks if tool is a write tool
2. If yes, computes diff (existing logic from `buildApprovalRequest`)
3. Calls `adk.StatefulInterrupt(ctx, &ApprovalRequest{...}, &approvalState{...})`
4. ADK serializes full agent state via `CheckPointStore`
5. SSE emits interrupt event to client
6. Client approves/rejects via REST endpoint
7. `runner.Resume()` restores state and continues (or feeds rejection result)

### Key files

| File | Lines | Change |
|------|-------|--------|
| `internal/agent/approval.go` | 83 | Replaced by interrupt/resume in middleware |
| `internal/agent/runner.go` | 181 | Resume logic added, checkpoint store wired |
| `internal/agent/service.go` | - | `activeApprovals sync.Map` removed |
| New: `internal/agent/checkpoint.go` | ~50 | SurrealDB CheckPointStore |
| New: DB migration | - | `agent_checkpoint` table |

### Effort: M

Depends on Phase 3. The approval middleware from Phase 3 transitions from channel-based to interrupt-based.

---

## Phase 5: Session Management + GenModelInput

**Goal**: Use eino's session system with DB-backed hydration for system prompt templating.

### What changes

- System prompt becomes a template with session value placeholders
- `DBSessionMiddleware.BeforeAgent` hydrates session values from DB:
  - `FolderTree` — vault folder structure
  - `Labels` — vault labels with counts
  - `VaultID` — for tool scoping
- Custom `GenModelInput` handles edge cases (instruction may contain JSON examples with curly braces)

### Implementation

```go
func (m *DBSessionMiddleware) BeforeAgent(ctx context.Context, agentCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
    folders, _ := m.db.ListFolders(ctx, m.vaultID)
    labels, _ := m.db.ListLabelsWithCounts(ctx, m.vaultID)

    adk.AddSessionValues(ctx, map[string]any{
        "FolderTree": formatFolderTree(folders),
        "Labels":     formatLabels(labels),
    })
    return ctx, agentCtx, nil
}
```

Instruction template:

```
You are a helpful knowledge assistant...

Vault folder structure:
{FolderTree}

Vault labels:
{Labels}
```

### Key files

| File | Lines | Change |
|------|-------|--------|
| `internal/agent/service.go:93-137` | 45 | buildSystemPrompt() replaced by template + middleware |

### Effort: S

Natural extension of Phase 3 middleware. Can be done in parallel with Phase 4.

---

## Phase 6: compose.Chain for RAG Pipeline

**Goal**: Make search composable — enable LLM reranking and query expansion.

### What changes

`search/service.go` Search() decomposed into chain steps:

```
QueryExpansion (optional, HyDE) → BM25Search → VectorSearch → RRFFusion → Reranking (optional) → ResultAssembly
```

Each step is a `compose.Lambda` wrapping existing logic:

```go
chain := compose.NewChain[SearchInput, []SearchResult]()
chain.AppendLambda(bm25Lambda)
chain.AppendLambda(vectorLambda)
chain.AppendLambda(rrfFusionLambda)
if reranking {
    chain.AppendLambda(rerankLambda) // uses ChatModel
}
chain.AppendLambda(assemblyLambda)
```

### Benefits

- Each step independently testable
- HyDE query expansion toggleable via chain composition
- LLM reranking toggleable
- Callbacks automatically instrument each step

### Key files

| File | Lines | Change |
|------|-------|--------|
| `internal/search/service.go` | 441 | Decomposed into chain steps |

### Effort: M

Current search works fine. This is enhancement — do after Phase 3 stabilizes.

---

## Phase 7 (Future): Multi-Agent Patterns

**Goal**: Leverage eino's agent hierarchy for complex workflows.

### Possible patterns

- **Supervisor agent**: routes to specialist agents (search specialist, writing specialist, memory specialist)
- **Plan-execute** (`adk/prebuilt/planexecute`): multi-step research ("research X across 5 documents and write a summary")
- **Deep agent** (`adk/prebuilt/deep`): complex knowledge synthesis with task decomposition
- **TransferToAgent**: hand off between agents based on intent

### Evaluation criteria

- Wait until Phases 1-6 are stable before evaluating
- Measure whether single-agent performance is sufficient for current use cases
- Multi-agent adds complexity — only adopt if measurable quality improvement

### Effort: L

Future work.

---

## Implementation Order

```
Phase 1: InvokableTool ✅ ───┐
                              ├─→ Phase 3: ADK ChatModelAgent ✅ ─┬─→ Phase 4: Interrupt/Resume
Phase 2: Callbacks ✅ ───────┘                                    └─→ Phase 5: Session + GenModelInput
                                                                       Phase 6: RAG Chain (independent)
                                                                       Phase 7: Multi-Agent (future)
```

- **Phases 1 + 2 + 3**: ✅ complete
- **Phases 4 + 5**: can run in parallel, next up
- **Phase 6**: independent, after Phase 3 stabilizes
- **Phase 7**: future, after all above stable

---

## Key Files Summary

| File | Lines | Role | Phases |
|------|-------|------|--------|
| `internal/agent/service.go` | ~640 | ADK agent construction, event consumption, persistence | ✅ 3, 5 |
| `internal/agent/middleware.go` | ~330 | ADK middleware + sessionDumpAgent wrapper | ✅ 3 |
| `internal/agent/events.go` | ~70 | Custom event types, TokenUsage, ToolRecord, AgentResult | ✅ 3 |
| `internal/agent/websearch_tool.go` | ~60 | WebSearchTool InvokableTool | ✅ 3 |
| `internal/llm/model.go` | ~545 | LLM wrapper, BaseChatModel() accessor | ✅ 2, 3 |
| `internal/tools/executor.go` | ~110 | Registry-based tool dispatch via InvokableTool | ✅ 1 |
| `internal/tools/tool_*.go` | ~900 | Individual InvokableTool implementations (10 tools) | ✅ 1 |
| `internal/tools/meta.go` | ~40 | Context-based ToolResultMeta passing (SetResultMeta exported) | ✅ 1, 3 |
| `internal/mcptools/tools.go` | 561 | MCP tool bridge (shares executor) | ✅ 1 (unchanged) |
| `internal/search/service.go` | 441 | Hybrid BM25+vector search | 6 |
| `internal/agent/handler.go` | 324 | REST API endpoints | ✅ 3 (unchanged) |
| `internal/metrics/collector.go` | 203 | Metrics collection | ✅ 2 |
| `internal/agent/runner.go` | 181 | Background execution + SSE event replay | ✅ 3, 4 |
| `internal/agent/approval.go` | 83 | In-memory approval registry | 4 |

---

## Lines Eliminated (Estimated)

| Phase | Lines Removed | Lines Added | Net |
|-------|--------------|-------------|-----|
| Phase 1 ✅ | 699 (executor switch + buildTools + name mapping) | 63 (modified) + ~900 (tool structs + infra) | ~-636 net in modified files |
| Phase 2 ✅ | 0 (additive — manual timing kept for providers without callbacks) | ~60 (callback handler) + ~50 (tests) | +110 |
| Phase 3 ✅ | 376 (Chat loop + GenerateStreamWithTools + buildTools + executeTool) | 515 (middleware + events + agent wiring) | +139 |
| Phase 4 | ~83 (approval registry) | ~80 (checkpoint store + interrupt) | ~0 |
| Phase 5 | ~45 (buildSystemPrompt) | ~30 (session middleware) | -15 |
| **Total** | **~1158** | **~1588** | **+430** |

Net increase of ~430 lines, but the new code is composable, testable middleware with proper error handling and race protection — replacing monolithic orchestration.
