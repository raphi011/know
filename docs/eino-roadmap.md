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
| `schema` | `Message`, `ToolCall`, `ToolInfo`, `ParameterInfo`, `NewParamsOneOfByParams`, `TokenUsage`, `ConcatMessages`, `StreamReader`, `MessageInputPart`, `RoleType` constants |
| `components/model` | `BaseChatModel` (Generate, Stream), `model.WithMaxTokens`, `model.WithTools` |
| `components/embedding` | `Embedder` (EmbedStrings) |
| `eino-ext` | `model/claude`, `model/openai`, `model/gemini`, `model/ollama`, `embedding/*` |

### What we do NOT use

| eino Feature | Status |
|-------------|--------|
| ADK (`ChatModelAgent`, `Runner`) | Not used — custom agent loop |
| `compose.*` (Graph, Chain, ToolsNode) | Not used — manual tool dispatch |
| `callbacks.Handler` | Not used — manual slog + metrics |
| Interrupt/Resume, CheckPointStore | Not used — in-memory approval channels |
| Session management (AddSessionValue) | Not used — manual prompt building |
| `tool.InvokableTool` | Not used — raw `ToolInfo` + switch dispatch |
| `retriever.Retriever`, `indexer.Indexer` | Not used — direct DB queries |

---

## What Our Custom Code Does (to be replaced)

| Custom Code | File | Lines | Eino Replacement |
|-------------|------|-------|-----------------|
| Agent loop (Chat method) | `agent/service.go` | ~290 | ADK `ChatModelAgent` ReAct loop |
| Tool calling loop | `llm/model.go` GenerateStreamWithTools | ~110 | ADK handles internally |
| Tool dispatch (sequential switch) | `tools/executor.go` ExecuteTool | ~60 | `compose.ToolsNode` (parallel) |
| Tool definitions | `agent/service.go` buildTools | ~145 | `tool.InvokableTool` implementations |
| System prompt building | `agent/service.go` buildSystemPrompt | ~45 | `GenModelInput` + session values |
| Message history assembly | `agent/service.go` buildMessages | ~25 | `ChatModelAgentState.Messages` |
| Doc ref / attachment injection | `agent/service.go` Chat lines 386-435 | ~50 | `BeforeModelRewriteState` middleware |
| Approval workflow | `agent/approval.go` | 83 | Interrupt/Resume + `WrapInvokableToolCall` |
| Token tracking (manual) | `llm/model.go` extractTokenCounts | ~30 | Callbacks + `ResponseMeta` (automatic) |
| Manual LLM observability | `llm/model.go` scattered | ~40 | Callbacks system |

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

- `ToolResultMeta` returned via context (`WithResultMeta`/`ResultMeta`/`setResultMeta`) since `InvokableRun` returns `(string, error)`
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

## Phase 2: Callbacks System

**Goal**: Replace manual observability with eino's callback handlers.

### What changes

- Global callback handler that logs via `logutil.FromCtx(ctx)` (preserves structured logging)
- Per-component timing replaces manual `time.Now()` / `defer logOp()` in LLM calls
- Token usage tracking automatic via `ResponseMeta` callbacks
- Metrics integration: callback handler calls `metrics.RecordLLMUsage` on `OnEnd`
- Foundation for OpenTelemetry traces later

### Implementation

```go
type observabilityHandler struct{}

func (h *observabilityHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
    logutil.FromCtx(ctx).Debug("component starting", "component", info.Name, "type", info.Type)
    return context.WithValue(ctx, startTimeKey{}, time.Now())
}

func (h *observabilityHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
    start := ctx.Value(startTimeKey{}).(time.Time)
    logutil.FromCtx(ctx).Debug("component complete", "component", info.Name, "duration_ms", time.Since(start).Milliseconds())
    // Extract token usage from model output if applicable
    return ctx
}
```

Register globally at startup:

```go
callbacks.AppendGlobalHandlers(&observabilityHandler{})
```

### Key files

| File | Lines | Change |
|------|-------|--------|
| `internal/llm/model.go` | 691 | Manual timing + metrics removed from Generate/Stream methods |
| `internal/metrics/collector.go` | 203 | Called from callback handler instead of inline |
| New: `internal/llm/callbacks.go` | ~60 | Observability callback handler |

### Effort: S

Additive, no behavioral changes. Can be done in parallel with Phase 1.

---

## Phase 3: ADK ChatModelAgent Migration

**Goal**: Replace the custom agent loop with `ChatModelAgent`. This is the biggest change.

### What changes

- `agent.Service.Chat()` (~290 lines) replaced by `ChatModelAgent.Run()` via `adk.Runner`
- `llm.Model.GenerateStreamWithTools()` (~110 lines) eliminated — ADK handles the ReAct loop
- System prompt via `ChatModelAgentConfig.Instruction` + `GenModelInput`
- Message history: ADK manages `ChatModelAgentState.Messages` internally
- SSE streaming via `AsyncIterator[*AgentEvent]` → our SSE event emitter
- Max iterations via `ChatModelAgentConfig.MaxIterations` (replaces `const maxIterations = 10`)
- Retry with backoff via `ModelRetryConfig`

### Middleware stack

Each middleware is a separate struct embedding `*adk.BaseChatModelAgentMiddleware`:

| # | Middleware | Hook | Purpose |
|---|-----------|------|---------|
| 1 | `DBSessionMiddleware` | `BeforeAgent` | Hydrate session values from DB (folders, labels) for system prompt interpolation |
| 2 | `DocRefMiddleware` | `BeforeModelRewriteState` | Inject doc refs + text attachments as messages before each model call |
| 3 | `ImageAttachmentMiddleware` | `BeforeModelRewriteState` | Build multimodal user messages with image attachments |
| 4 | `ApprovalMiddleware` | `WrapInvokableToolCall` | Gate write tools on user approval (Phase 4 upgrades to interrupt/resume) |
| 5 | `PersistenceMiddleware` | `AfterModelRewriteState` | Persist assistant messages + tool call records to SurrealDB |
| 6 | `TokenTrackingMiddleware` | `AfterModelRewriteState` | Accumulate + persist token usage on conversation |

SSE streaming is handled by iterating the `AsyncIterator[*AgentEvent]` in the REST handler and mapping `AgentEvent` types to our `StreamEvent` types.

### Agent construction

```go
agent, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Name:        "knowhow-assistant",
    Instruction: baseInstruction, // static part
    Model:       chatModel,
    ToolsConfig: adk.ToolsConfig{
        ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools: tools, // []tool.BaseTool from Phase 1
        },
    },
    GenModelInput: customGenModelInput, // avoids FString conflicts
    MaxIterations: 10,
    Handlers: []adk.ChatModelAgentMiddleware{
        dbSessionMiddleware,
        docRefMiddleware,
        imageAttachmentMiddleware,
        approvalMiddleware,
        persistenceMiddleware,
        tokenTrackingMiddleware,
    },
    ModelRetryConfig: &adk.ModelRetryConfig{...},
})

runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:          agent,
    EnableStreaming: true,
})
```

### SSE event mapping

```go
iter := runner.Run(ctx, messages)
for {
    event, err := iter.Recv()
    if err == io.EOF { break }
    if event.Output != nil && event.Output.MessageOutput != nil {
        mv := event.Output.MessageOutput
        if mv.IsStreaming {
            // Stream tokens to SSE
        } else {
            // Final message
        }
    }
    if event.Action != nil && event.Action.Interrupted != nil {
        // Approval required (Phase 4)
    }
}
```

### Key files

| File | Lines | Change |
|------|-------|--------|
| `internal/agent/service.go` | 804 | Chat() rewritten, buildTools/buildMessages/buildSystemPrompt removed |
| `internal/llm/model.go` | 691 | GenerateStreamWithTools() removed |
| `internal/agent/handler.go` | 324 | REST handler adapts to AsyncIterator |
| `internal/agent/runner.go` | 181 | Background execution adapts to ADK Runner |
| New: `internal/agent/middleware.go` | ~200 | All middleware implementations |

### Risks & mitigations

| Risk | Mitigation |
|------|-----------|
| SSE streaming granularity — ADK `AsyncIterator` may emit per-message not per-token | ADK's `eventSenderModelWrapper` sends streaming events; iterate inner `MessageStream` for per-token SSE |
| Multimodal messages | `AgentInput.Messages` supports `UserInputMultiContent` — should work as-is |
| Token accumulation across iterations | ADK tracks per-iteration via `ResponseMeta`; use `AfterModelRewriteState` to accumulate |
| MCP tools sharing | MCP server uses `ToolExecutor` interface independently — decouple from agent-specific middleware |
| Auto-title on first message | Move to `AfterModelRewriteState` or keep as a post-run goroutine |

### Effort: L

Biggest change — eliminates ~500 lines of custom orchestration. Depends on Phase 1.

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
Phase 1: InvokableTool + ToolsNode ─────┐
                                         ├─→ Phase 3: ADK ChatModelAgent ─┬─→ Phase 4: Interrupt/Resume
Phase 2: Callbacks ─────────────────────┘                                 └─→ Phase 5: Session + GenModelInput
                                                                               Phase 6: RAG Chain (independent)
                                                                               Phase 7: Multi-Agent (future)
```

- **Phases 1 + 2**: can run in parallel (no dependencies)
- **Phase 3**: depends on Phase 1 (tools must be InvokableTool)
- **Phases 4 + 5**: can run in parallel after Phase 3
- **Phase 6**: independent, after Phase 3 stabilizes
- **Phase 7**: future, after all above stable

---

## Key Files Summary

| File | Lines | Role | Phases |
|------|-------|------|--------|
| `internal/agent/service.go` | ~650 | Custom agent loop, system prompt, message assembly | 3, 5 |
| `internal/llm/model.go` | 691 | LLM wrapper, GenerateStreamWithTools, token tracking | 2, 3 |
| `internal/tools/executor.go` | ~110 | Registry-based tool dispatch via InvokableTool | ✅ 1 |
| `internal/tools/tool_*.go` | ~900 | Individual InvokableTool implementations (10 tools) | ✅ 1 |
| `internal/mcptools/tools.go` | 561 | MCP tool bridge (shares executor) | ✅ 1 (unchanged) |
| `internal/search/service.go` | 441 | Hybrid BM25+vector search | 6 |
| `internal/agent/handler.go` | 324 | REST API endpoints | 3 |
| `internal/metrics/collector.go` | 203 | Metrics collection | 2 |
| `internal/agent/runner.go` | 181 | Background execution + SSE event replay | 3, 4 |
| `internal/agent/approval.go` | 83 | In-memory approval registry | 4 |

---

## Lines Eliminated (Estimated)

| Phase | Lines Removed | Lines Added | Net |
|-------|--------------|-------------|-----|
| Phase 1 ✅ | 699 (executor switch + buildTools + name mapping) | 63 (modified) + ~900 (tool structs + infra) | ~-636 net in modified files |
| Phase 2 | ~40 (manual timing) | ~60 (callback handler) | +20 |
| Phase 3 | ~500 (Chat loop + GenerateStreamWithTools) | ~200 (middleware + wiring) | -300 |
| Phase 4 | ~83 (approval registry) | ~80 (checkpoint store + interrupt) | ~0 |
| Phase 5 | ~45 (buildSystemPrompt) | ~30 (session middleware) | -15 |
| **Total** | **~1018** | **~670** | **~-350** |

Net reduction of ~350 lines, but more importantly: the remaining code is composable, testable middleware rather than monolithic orchestration.
