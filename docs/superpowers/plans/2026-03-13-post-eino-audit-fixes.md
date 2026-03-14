# Post-Eino Migration Audit Fixes

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix bugs and simplify the agent data flow identified in the post-eino-migration audit.

**Architecture:** Three independent changes: (1) fix bare `slog` calls that bypass request-scoped logging, (2) eliminate `sessionDumpAgent` wrapper by moving token/tool tracking to middleware struct fields, (3) add configurable agent timeout.

**Tech Stack:** Go, eino ADK middlewares, SurrealDB

---

## File Structure

| File | Action | Purpose |
|------|--------|---------|
| `internal/agent/middleware.go` | Modify | Add struct fields for token/tool tracking, remove session value usage, delete `sessionDumpAgent` |
| `internal/agent/service.go` | Modify | Return middleware pointers from `buildAgent`, extract data after iter drains, add timeout |
| `internal/agent/events.go` | Modify | Delete `RunCompleteEvent`, session value keys, extract helpers |
| `internal/agent/runner.go` | Modify | Pass timeout, extract data from middlewares |
| `internal/agent/approval_tool.go` | Modify | Fix `slog.Warn` → `logutil.FromCtx(ctx)` |
| `internal/search/service.go` | Modify | Thread ctx through `assembleResults`, fix `slog.Warn` |
| `internal/search/service_test.go` | Modify | Update `assembleResults` call sites to pass ctx |

---

## Chunk 1: Bug Fixes + Data Flow Simplification

### Task 1: Fix bare slog calls

**Files:**
- Modify: `internal/agent/approval_tool.go:143`
- Modify: `internal/search/service.go:208,229`
- Modify: `internal/search/service_test.go` (all `assembleResults` call sites)

- [ ] **Step 1: Fix `slog.Warn` in `approval_tool.go`**

In `wrapWriteToolsForApproval`, replace `slog.Warn` with `logutil.FromCtx(ctx)`:

```go
// Before (line 143):
slog.Warn("failed to get tool info for approval wrapping, leaving unwrapped", "error", err)

// After:
logutil.FromCtx(ctx).Warn("failed to get tool info for approval wrapping, leaving unwrapped", "error", err)
```

Remove `"log/slog"` from imports if no longer used.

- [ ] **Step 2: Thread ctx through `assembleResults` and fix `slog.Warn`**

Add `ctx context.Context` as first parameter to `assembleResults`:

```go
func assembleResults(ctx context.Context, chunks []db.ChunkWithScore, limit int, fullContent bool, degraded bool) []SearchResult {
```

Replace `slog.Warn` at line 229 with:
```go
logutil.FromCtx(ctx).Warn("failed to extract chunk document ID", "chunk_id", ch.ID, "error", err)
```

Add `"context"` to imports, remove `"log/slog"` from imports.

Update all 4 call sites in `service.go` to pass `ctx`:
```go
return assembleResults(ctx, chunks, limit, input.FullContent, false), nil
```

- [ ] **Step 3: Update test call sites**

All `assembleResults` calls in `service_test.go` get `context.Background()` as first arg:
```go
results := assembleResults(context.Background(), chunks, 10, false, false)
```

Add `"context"` to imports.

- [ ] **Step 4: Run tests**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/approval_tool.go internal/search/service.go internal/search/service_test.go
git commit -m "fix: use context-scoped logging instead of bare slog calls"
```

---

### Task 2: Eliminate sessionDumpAgent — move tracking to struct fields

The `sessionDumpAgent` wrapper exists solely to extract token usage and tool records from session values after the agent finishes. This adds complexity (goroutine relay, `NewAsyncIteratorPair`, panic recovery, custom event type) for something that can be done directly: store data in middleware struct fields and read them after the iterator is drained.

**Files:**
- Modify: `internal/agent/middleware.go`
- Modify: `internal/agent/service.go`
- Modify: `internal/agent/events.go`
- Modify: `internal/agent/runner.go`

- [ ] **Step 1: Add struct fields to middlewares**

In `tokenTrackingMiddleware`, add fields + mutex for token tracking:
```go
type tokenTrackingMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	mu    sync.Mutex
	usage TokenUsage
}
```

In `toolExecutionMiddleware`, add a `records` field (already has `mu`):
```go
type toolExecutionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	service *Service
	req     *ChatRequest
	mu      sync.Mutex
	records []ToolRecord
}
```

- [ ] **Step 2: Update tokenTrackingMiddleware.AfterModelRewriteState**

Replace session value read/write with struct field access:
```go
func (m *tokenTrackingMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last.ResponseMeta == nil || last.ResponseMeta.Usage == nil {
		return ctx, state, nil
	}

	u := last.ResponseMeta.Usage

	m.mu.Lock()
	m.usage.InputTokens += int64(u.PromptTokens)
	m.usage.OutputTokens += int64(u.CompletionTokens)
	m.usage.FinalPromptTokens = int64(u.PromptTokens)
	m.mu.Unlock()

	return ctx, state, nil
}
```

- [ ] **Step 3: Update toolExecutionMiddleware.recordTool**

Replace session value read/write with struct field append:
```go
func (m *toolExecutionMiddleware) recordTool(_ context.Context, call schema.ToolCall, result string, meta *tools.ToolResultMeta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, ToolRecord{Call: call, Result: result, Meta: meta})
}
```

- [ ] **Step 4: Delete sessionDumpAgent and helpers**

Delete from `middleware.go`:
- `sessionDumpAgent` struct and its `Run` method (lines 299-338)
- `extractTokenUsage` helper (lines 340-347)
- `extractToolRecords` helper (lines 349-356)

Remove `"github.com/cloudwego/eino/adk"` import from `middleware.go` ONLY IF no other references remain. (It's still used by `BaseChatModelAgentMiddleware`, `ChatModelAgentContext`, `ChatModelAgentState`, `ModelContext`, `InvokableToolCallEndpoint`, `ToolContext`, `SendEvent`, `AgentEvent`, `AgentOutput`, `AddSessionValues`, `GetSessionValues`, `GetSessionValue` — several remain, so keep the import.)

- [ ] **Step 5: Update buildAgent to return middleware pointers**

Change `buildAgent` signature to return the middleware pointers alongside the runner:
```go
func (s *Service) buildAgent(ctx context.Context, model *llm.Model, req *ChatRequest, emit func(StreamEvent)) (*adk.Runner, *tokenTrackingMiddleware, *toolExecutionMiddleware, error) {
```

Remove the `sessionDumpAgent` wrapping — pass `agent` directly to `adk.NewRunner`:
```go
return adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           agent,
    EnableStreaming:  true,
    CheckPointStore: s.checkpointStore,
}), tokenMW, toolMW, nil
```

- [ ] **Step 6: Update Chat() to extract data from middlewares**

```go
// 5. Build agent + runner
runner, tokenMW, toolMW, err := s.buildAgent(ctx, model, &req, emit)
if err != nil {
    return fmt.Errorf("build agent: %w", err)
}

// 6. Run + consume
iter := runner.Run(ctx, messages, adk.WithCheckPointID(req.ConversationID))
result := consumeAgentEvents(ctx, iter, emit)

// 7. Populate result from middleware state
tokenMW.mu.Lock()
result.TokenUsage = tokenMW.usage
tokenMW.mu.Unlock()
toolMW.mu.Lock()
result.ToolRecords = toolMW.records
toolMW.mu.Unlock()
```

- [ ] **Step 7: Update ResumeChat() in runner.go similarly**

```go
adkRunner, tokenMW, toolMW, err := s.buildAgent(ctx, model, chatReq, emit)
if err != nil {
    return fmt.Errorf("build agent for resume: %w", err)
}

iter, err := adkRunner.ResumeWithParams(ctx, req.ConversationID, &adk.ResumeParams{
    Targets: map[string]any{
        req.InterruptID: &req.Response,
    },
})
if err != nil {
    return fmt.Errorf("resume from checkpoint: %w", err)
}

result := consumeAgentEvents(ctx, iter, emit)

tokenMW.mu.Lock()
result.TokenUsage = tokenMW.usage
tokenMW.mu.Unlock()
toolMW.mu.Lock()
result.ToolRecords = toolMW.records
toolMW.mu.Unlock()

return s.finalizeRun(ctx, req.ConversationID, model, &result, emit)
```

- [ ] **Step 8: Remove RunCompleteEvent handling from consumeAgentEvents**

Delete the `case *RunCompleteEvent:` block (lines 378-380) from the type switch. Also delete the `default:` case since ToolStartEvent and ToolEndEvent are the only remaining custom event types.

Actually, keep the `default:` case for forward-compatibility logging.

- [ ] **Step 9: Clean up events.go**

Delete from `events.go`:
- `sessionKeyTokenUsage` and `sessionKeyToolRecords` constants
- `RunCompleteEvent` struct

Remove unused imports if any.

- [ ] **Step 10: Run tests**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 11: Commit**

```bash
git add internal/agent/middleware.go internal/agent/service.go internal/agent/events.go internal/agent/runner.go
git commit -m "refactor: eliminate sessionDumpAgent, move tracking to middleware struct fields

Replace session-value-based token/tool record accumulation with direct
struct fields on the per-request middleware instances. This removes the
sessionDumpAgent wrapper (goroutine relay + NewAsyncIteratorPair +
RunCompleteEvent), simplifying the data flow from:

  tools → session values → sessionDumpAgent → RunCompleteEvent → consumer

to:

  tools → middleware struct fields → read after iter drained"
```

---

### Task 3: Add configurable agent timeout

**Files:**
- Modify: `internal/agent/service.go`

- [ ] **Step 1: Add timeout constant and apply in Chat()**

Add a constant near the top of `service.go`:
```go
const defaultAgentTimeout = 10 * time.Minute
```

In `Chat()`, wrap the context with a timeout before running the agent (after step 5, before step 6):
```go
// 6. Run with timeout
ctx, cancel := context.WithTimeout(ctx, defaultAgentTimeout)
defer cancel()

iter := runner.Run(ctx, messages, adk.WithCheckPointID(req.ConversationID))
```

- [ ] **Step 2: Apply same timeout in ResumeChat()**

In `ResumeChat()`, wrap context before `ResumeWithParams`:
```go
ctx, cancel := context.WithTimeout(ctx, defaultAgentTimeout)
defer cancel()

iter, err := adkRunner.ResumeWithParams(ctx, req.ConversationID, &adk.ResumeParams{
```

- [ ] **Step 3: Add `"time"` to imports if not already present**

- [ ] **Step 4: Run tests**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/service.go
git commit -m "fix: add 10-minute timeout to agent runs to prevent indefinite hangs"
```
