# Agent Tool Approval Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add write tools (create/edit document) to the internal agent and MCP server, with an inline approval mechanism that pauses agent execution until the user approves or rejects the diff.

**Architecture:** The agent's `onToolCall` callback checks if a tool is a write tool. If so (and approval is required), it emits a `tool_approval_required` SSE event with the diff, then blocks on a channel until the user responds via a new HTTP endpoint. The frontend renders the diff inline in the chat and sends the approval response. MCP tools do simple direct writes (MCP clients handle their own approval).

**Tech Stack:** Go (agent service, diff, HTTP handler), Next.js (SSE event handling, diff UI), SurrealDB (no schema changes needed)

---

### Task 1: Move diff utilities out of review package

The `review` package will be removed, but `ComputeHunks`, `ApplyHunks`, `ComputeStats`, and the `Hunk`/`DiffLine`/`DiffStats` types are needed by the agent. Move them to a standalone `internal/diff` package.

**Files:**
- Create: `internal/diff/diff.go` (move from `internal/review/diff.go`)
- Create: `internal/diff/diff_test.go` (move from `internal/review/diff_test.go`)
- Modify: `internal/review/service.go` — update imports to use `diff` package
- Modify: `internal/graph/helpers.go` — update imports if referencing review diff types

**Step 1: Create `internal/diff/diff.go`**

Copy `internal/review/diff.go` to `internal/diff/diff.go`. Change package declaration from `review` to `diff`. The types and functions stay identical:

```go
package diff

// All types (DiffLineType, DiffLine, Hunk, DiffStats) and functions
// (ComputeHunks, ApplyHunks, ComputeStats, splitLines) stay the same.
// Only the package name changes.
```

**Step 2: Move `internal/review/diff_test.go` to `internal/diff/diff_test.go`**

Change package from `review_test` to `diff_test`, update imports.

**Step 3: Update `internal/review/service.go` imports**

Replace direct usage of `Hunk`, `ComputeHunks`, `ApplyHunks`, `ComputeStats`, `DiffResult` etc. with imports from `github.com/raphi011/knowhow/internal/diff`. The `DiffResult` struct stays in `review/service.go` since it's review-specific, but its `Hunks` field type changes to `diff.Hunk`.

**Step 4: Update GraphQL helpers if needed**

Check `internal/graph/helpers.go` for any references to review diff types and update to `diff` package.

**Step 5: Run tests**

Run: `just test`
Expected: All tests pass

**Step 6: Commit**

```bash
git add internal/diff/ internal/review/ internal/graph/
git commit -m "refactor: extract diff utilities into standalone package"
```

---

### Task 2: Add approval types and channel management

Create the approval infrastructure — types, channel management, and the approval request/response structs.

**Files:**
- Create: `internal/agent/approval.go`

**Step 1: Create `internal/agent/approval.go`**

```go
package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/raphi011/knowhow/internal/diff"
)

// ApprovalAction is the user's decision on a tool approval request.
type ApprovalAction string

const (
	ApprovalApproveAll   ApprovalAction = "approve_all"
	ApprovalApproveHunks ApprovalAction = "approve_hunks"
	ApprovalReject       ApprovalAction = "reject"
)

// ApprovalRequest is emitted via SSE when a write tool needs user approval.
type ApprovalRequest struct {
	CallID string     `json:"callId"`
	Tool   string     `json:"tool"`
	Path   string     `json:"path"`
	IsNew  bool       `json:"isNew"`
	Diff   *DiffPayload `json:"diff,omitempty"` // nil for new documents
	Content string    `json:"content,omitempty"` // full content for new documents
}

// DiffPayload contains the computed diff for an edit.
type DiffPayload struct {
	Hunks []diff.Hunk  `json:"hunks"`
	Stats diff.DiffStats `json:"stats"`
}

// ApprovalResponse is sent by the user to approve or reject a tool call.
type ApprovalResponse struct {
	CallID      string         `json:"callId"`
	Action      ApprovalAction `json:"action"`
	HunkIndexes []int          `json:"hunkIndexes,omitempty"`
}

// approvalRegistry manages pending approval channels keyed by call ID.
type approvalRegistry struct {
	mu       sync.Mutex
	pending  map[string]chan ApprovalResponse
}

func newApprovalRegistry() *approvalRegistry {
	return &approvalRegistry{pending: make(map[string]chan ApprovalResponse)}
}

// register creates a channel for the given call ID and returns it.
func (r *approvalRegistry) register(callID string) <-chan ApprovalResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan ApprovalResponse, 1)
	r.pending[callID] = ch
	return ch
}

// resolve sends a response to the waiting goroutine and removes the entry.
func (r *approvalRegistry) resolve(resp ApprovalResponse) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.pending[resp.CallID]
	if !ok {
		return fmt.Errorf("no pending approval for call %q", resp.CallID)
	}
	ch <- resp
	delete(r.pending, resp.CallID)
	return nil
}

// cancel removes all pending approvals (e.g. on context cancellation).
func (r *approvalRegistry) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, ch := range r.pending {
		close(ch)
		delete(r.pending, id)
	}
}
```

**Step 2: Verify it compiles**

Run: `just build-all`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add internal/agent/approval.go
git commit -m "feat(agent): add approval types and channel registry"
```

---

### Task 3: Add write tools to agent service

Add `create_document` and `edit_document` to the agent's tool list and implement their execution with approval gating.

**Files:**
- Modify: `internal/agent/service.go` — add tools to `buildTools()`, add to `executeTool()`, add approval flow to `Chat()`

**Step 1: Add document service dependency**

Add `docService *document.Service` to the `Service` struct and `NewService` constructor. Import `"github.com/raphi011/knowhow/internal/document"`.

In `service.go`, update:
```go
type Service struct {
	db         *db.Client
	model      *llm.Model
	search     *search.Service
	docService *document.Service
	tavily     *tavilyClient
}

func NewService(db *db.Client, model *llm.Model, search *search.Service, docService *document.Service, tavilyAPIKey string) *Service {
	return &Service{
		db:         db,
		model:      model,
		search:     search,
		docService: docService,
		tavily:     newTavilyClient(tavilyAPIKey),
	}
}
```

Update all call sites of `NewService` (check `cmd/knowhow-server/main.go` or wherever the agent service is constructed).

**Step 2: Add tool definitions to `buildTools()`**

Append to the tools slice in `buildTools()` (around line 168, before the tavily conditional):

```go
{
	Name: "create_document",
	Desc: "Create a new document in the knowledge base. The content should be markdown. Fails if a document already exists at the given path.",
	ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"path": {
			Type:     schema.String,
			Desc:     "Document path (e.g. /guides/new-guide.md)",
			Required: true,
		},
		"content": {
			Type:     schema.String,
			Desc:     "Full markdown content for the document",
			Required: true,
		},
	}),
},
{
	Name: "edit_document",
	Desc: "Edit an existing document by replacing its full content. Read the document first to get the current content, then modify and pass the complete new content.",
	ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
		"path": {
			Type:     schema.String,
			Desc:     "Document path of the existing document",
			Required: true,
		},
		"content": {
			Type:     schema.String,
			Desc:     "Complete new markdown content (replaces existing content entirely)",
			Required: true,
		},
	}),
},
```

**Step 3: Add `executeTool` cases for write tools**

In `executeTool()`, add cases for the new tools. These perform the actual write (called after approval or in auto-approve mode):

```go
case "create_document":
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", nil, fmt.Errorf("parse create_document args: %w", err)
	}
	if args.Path == "" || args.Content == "" {
		return "", nil, fmt.Errorf("path and content are required")
	}

	start := time.Now()
	doc, err := s.docService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    args.Path,
		Content: args.Content,
		Source:  models.SourceAIGenerated,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create document: %w", err)
	}
	dur := time.Since(start).Milliseconds()
	return fmt.Sprintf("Document created at %s", doc.Path), &ToolResultMeta{
		DurationMs:   dur,
		DocumentPath: &doc.Path,
		DocumentTitle: &doc.Title,
	}, nil

case "edit_document":
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", nil, fmt.Errorf("parse edit_document args: %w", err)
	}
	if args.Path == "" || args.Content == "" {
		return "", nil, fmt.Errorf("path and content are required")
	}

	start := time.Now()
	doc, err := s.docService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    args.Path,
		Content: args.Content,
		Source:  models.SourceAIGenerated,
	})
	if err != nil {
		return "", nil, fmt.Errorf("edit document: %w", err)
	}
	dur := time.Since(start).Milliseconds()
	return fmt.Sprintf("Document updated at %s", doc.Path), &ToolResultMeta{
		DurationMs:   dur,
		DocumentPath: &doc.Path,
		DocumentTitle: &doc.Title,
	}, nil
```

**Step 4: Verify build**

Run: `just build-all`
Expected: Build succeeds (may need to update the call site first)

**Step 5: Commit**

```bash
git add internal/agent/service.go cmd/
git commit -m "feat(agent): add create_document and edit_document tools"
```

---

### Task 4: Implement approval gating in the agent loop

Wire the approval channel into the `onToolCall` callback so write tools pause for user approval.

**Files:**
- Modify: `internal/agent/service.go` — update `Chat()` method
- Modify: `internal/agent/handler.go` — pass approval mode and registry

**Step 1: Add `ChatRequest` fields**

Add approval-related fields to `ChatRequest`:

```go
type ChatRequest struct {
	ConversationID string
	VaultID        string
	UserID         string
	Content        string
	DocRefs        []string
	AutoApprove    bool              // true = skip approval for write tools
	Approvals      *approvalRegistry // nil if auto-approve
}
```

**Step 2: Update `onToolCall` in `Chat()` method**

In the `onToolCall` callback (around line 314), add a check before executing write tools:

```go
func(call schema.ToolCall) (string, error) {
	var inputMap map[string]any
	if jsonErr := json.Unmarshal([]byte(call.Function.Arguments), &inputMap); jsonErr != nil {
		inputMap = map[string]any{"raw": call.Function.Arguments}
	}
	emit(StreamEvent{Type: "tool_start", CallID: call.ID, Tool: call.Function.Name, Input: inputMap})

	toolName := call.Function.Name

	// Check if this is a write tool that needs approval
	if isWriteTool(toolName) && !req.AutoApprove && req.Approvals != nil {
		approvalReq, buildErr := s.buildApprovalRequest(ctx, req.VaultID, call)
		if buildErr != nil {
			emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: fmt.Sprintf("error: %v", buildErr)})
			toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: fmt.Sprintf("error: %v", buildErr)})
			return fmt.Sprintf("error: %v", buildErr), nil
		}

		// Emit approval request and wait
		emit(StreamEvent{Type: "tool_approval_required", CallID: call.ID, Tool: toolName, Approval: approvalReq})
		ch := req.Approvals.register(call.ID)

		var resp ApprovalResponse
		select {
		case resp = <-ch:
		case <-ctx.Done():
			return "error: request cancelled", nil
		}

		if resp.Action == ApprovalReject {
			result := "User rejected the proposed changes."
			emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: result})
			toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result})
			return result, nil
		}

		// Handle partial hunk approval for edit_document
		if resp.Action == ApprovalApproveHunks && approvalReq.Diff != nil {
			merged, mergeErr := diff.ApplyHunks(/* original content */, approvalReq.Diff.Hunks, resp.HunkIndexes)
			if mergeErr != nil {
				errMsg := fmt.Sprintf("error applying hunks: %v", mergeErr)
				emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: errMsg})
				toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: errMsg})
				return errMsg, nil
			}
			// Override the content in arguments with merged content
			inputMap["content"] = merged
			newArgs, _ := json.Marshal(inputMap)
			call.Function.Arguments = string(newArgs)
		}
	}

	// Execute the tool
	result, meta, execErr := s.executeTool(ctx, req.VaultID, toolName, call.Function.Arguments)
	if execErr != nil {
		slog.Warn("tool execution failed", "tool", toolName, "error", execErr)
		emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Content: fmt.Sprintf("error: %v", execErr)})
		toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: fmt.Sprintf("error: %v", execErr), meta: nil})
		return fmt.Sprintf("error: %v", execErr), nil
	}

	emit(StreamEvent{Type: "tool_end", CallID: call.ID, Tool: toolName, Meta: meta})
	toolCallRecords = append(toolCallRecords, toolCallRecord{call: call, result: result, meta: meta})
	return result, nil
},
```

**Step 3: Add helper functions**

```go
// isWriteTool returns true for tools that modify documents.
func isWriteTool(name string) bool {
	return name == "create_document" || name == "edit_document"
}

// buildApprovalRequest computes the diff for a write tool call.
func (s *Service) buildApprovalRequest(ctx context.Context, vaultID string, call schema.ToolCall) (*ApprovalRequest, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	req := &ApprovalRequest{
		CallID: call.ID,
		Tool:   call.Function.Name,
		Path:   args.Path,
	}

	// Check if document exists
	doc, err := s.db.GetDocumentByPath(ctx, vaultID, args.Path)
	if err != nil {
		return nil, fmt.Errorf("check document: %w", err)
	}

	if doc == nil {
		// New document — show full content
		req.IsNew = true
		req.Content = args.Content
		return req, nil
	}

	// Existing document — compute diff
	hunks, err := diff.ComputeHunks(doc.Content, args.Content, 3)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}
	req.Diff = &DiffPayload{
		Hunks: hunks,
		Stats: diff.ComputeStats(hunks),
	}
	return req, nil
}
```

**Step 4: Update `StreamEvent` to carry approval data**

Add the `Approval` field to `StreamEvent`:

```go
type StreamEvent struct {
	Type     string           `json:"type"`
	Content  string           `json:"content,omitempty"`
	ConvID   string           `json:"convId,omitempty"`
	MsgID    string           `json:"msgId,omitempty"`
	CallID   string           `json:"callId,omitempty"`
	Tool     string           `json:"tool,omitempty"`
	Input    map[string]any   `json:"input,omitempty"`
	Meta     *ToolResultMeta  `json:"meta,omitempty"`
	Approval *ApprovalRequest `json:"approval,omitempty"`
}
```

**Step 5: Verify build**

Run: `just build-all`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): implement approval gating for write tools"
```

---

### Task 5: Add approval HTTP endpoint

Add an endpoint for the frontend to send approval responses.

**Files:**
- Modify: `internal/agent/handler.go` — add `HandleApproval()` method
- Modify: `internal/agent/service.go` — store active approval registries per conversation

**Step 1: Add active registries map to Service**

The service needs to track which conversations have active approval registries:

```go
type Service struct {
	db         *db.Client
	model      *llm.Model
	search     *search.Service
	docService *document.Service
	tavily     *tavilyClient

	activeApprovals sync.Map // map[conversationID]*approvalRegistry
}
```

In `Chat()`, store and clean up the registry:
```go
// At start of Chat(), before the LLM call:
approvals := newApprovalRegistry()
s.activeApprovals.Store(req.ConversationID, approvals)
defer func() {
	approvals.cancel()
	s.activeApprovals.Delete(req.ConversationID)
}()

// Pass to ChatRequest:
req.Approvals = approvals
```

**Step 2: Add `HandleApproval()` to handler.go**

```go
type approvalRequestBody struct {
	ConversationID string         `json:"conversationId"`
	CallID         string         `json:"callId"`
	Action         ApprovalAction `json:"action"`
	HunkIndexes    []int          `json:"hunkIndexes,omitempty"`
}

func (s *Service) HandleApproval() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var body approvalRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		val, ok := s.activeApprovals.Load(body.ConversationID)
		if !ok {
			http.Error(w, "no active approval session", http.StatusNotFound)
			return
		}
		registry := val.(*approvalRegistry)

		if err := registry.resolve(ApprovalResponse{
			CallID:      body.CallID,
			Action:      body.Action,
			HunkIndexes: body.HunkIndexes,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
```

**Step 3: Register the endpoint**

Find where `HandleChat()` is registered (likely in `cmd/knowhow-server/main.go` or a router setup file) and add the approval endpoint:

```go
mux.Handle("/agent/approval", agentService.HandleApproval())
```

**Step 4: Add Next.js API route**

Create `web/app/api/agent/approval/route.ts` to proxy approval requests to the backend (similar to the existing chat route):

```typescript
import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/app/lib/session";

export async function POST(req: NextRequest) {
  const session = await getSession();
  if (!session) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const body = await req.json();

  const res = await fetch(`${session.serverUrl}/agent/approval`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${session.token}`,
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const text = await res.text();
    return NextResponse.json({ error: text }, { status: res.status });
  }

  return NextResponse.json({ ok: true });
}
```

**Step 5: Verify build**

Run: `just build-all && cd web && bun run build`
Expected: Both builds succeed

**Step 6: Commit**

```bash
git add internal/agent/ cmd/ web/app/api/agent/approval/
git commit -m "feat(agent): add approval HTTP endpoint and API route"
```

---

### Task 6: Add MCP write tools

Add `create_document` and `edit_document` to the MCP server. These are direct writes — MCP clients handle tool approval natively.

**Files:**
- Modify: `cmd/knowhow-mcp/tools.go` — add tools and handlers

**Step 1: Add input types**

```go
type CreateDocumentInput struct {
	Path     string  `json:"path" jsonschema:"description=Document path (e.g. /guides/new-guide.md)"`
	Content  string  `json:"content" jsonschema:"description=Full markdown content"`
	Instance string  `json:"instance" jsonschema:"description=Target instance name"`
}

type EditDocumentInput struct {
	Path     string  `json:"path" jsonschema:"description=Document path of the existing document"`
	Content  string  `json:"content" jsonschema:"description=Complete new markdown content (replaces existing)"`
	Instance string  `json:"instance" jsonschema:"description=Target instance name"`
}
```

**Step 2: Register tools in `register()`**

```go
mcp.AddTool(server, &mcp.Tool{
	Name:        "create_document",
	Description: "Create a new document in the knowledge base. Content should be markdown. Fails if a document already exists at the path.",
}, t.createDocument)

mcp.AddTool(server, &mcp.Tool{
	Name:        "edit_document",
	Description: "Edit an existing document by replacing its full content. Read the document first with get_document, then pass the complete new content.",
}, t.editDocument)
```

**Step 3: Implement handlers**

```go
func (t *mcpTools) createDocument(ctx context.Context, req *mcp.CallToolRequest, input CreateDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}

	instances := t.filterInstances(&input.Instance)
	if len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", input.Instance)
	}
	inst := instances[0]

	if err := inst.resolveVaults(ctx); err != nil {
		return nil, nil, fmt.Errorf("resolve vaults: %w", err)
	}

	vaultID := inst.vaultIDs[0]
	vars := map[string]any{
		"vaultId": vaultID,
		"file": map[string]any{
			"path":    input.Path,
			"content": input.Content,
		},
		"source": "mcp",
	}

	var resp createDocumentResponse
	if err := inst.client.Do(ctx, `mutation CreateDoc($vaultId: ID!, $file: FileInput!, $source: String) { createDocument(vaultId: $vaultId, file: $file, source: $source) { path } }`, vars, &resp); err != nil {
		return nil, nil, fmt.Errorf("create document: %w", err)
	}

	return textResult(fmt.Sprintf("Document created at %s (instance: %s)", resp.CreateDocument.Path, inst.name)), nil, nil
}

func (t *mcpTools) editDocument(ctx context.Context, req *mcp.CallToolRequest, input EditDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}

	instances := t.filterInstances(&input.Instance)
	if len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", input.Instance)
	}
	inst := instances[0]

	if err := inst.resolveVaults(ctx); err != nil {
		return nil, nil, fmt.Errorf("resolve vaults: %w", err)
	}

	vaultID := inst.vaultIDs[0]
	vars := map[string]any{
		"vaultId": vaultID,
		"path":    input.Path,
		"content": input.Content,
	}

	var resp struct {
		UpdateDocument struct {
			Path string `json:"path"`
		} `json:"updateDocument"`
	}
	if err := inst.client.Do(ctx, `mutation UpdateDoc($vaultId: ID!, $path: String!, $content: String!) { updateDocument(vaultId: $vaultId, path: $path, content: $content) { path } }`, vars, &resp); err != nil {
		return nil, nil, fmt.Errorf("edit document: %w", err)
	}

	return textResult(fmt.Sprintf("Document updated at %s (instance: %s)", resp.UpdateDocument.Path, inst.name)), nil, nil
}
```

**Step 4: Verify build**

Run: `just build-all`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add cmd/knowhow-mcp/tools.go
git commit -m "feat(mcp): add create_document and edit_document tools"
```

---

### Task 7: Frontend — handle `tool_approval_required` SSE event

Update the chat reducer and context to handle the new SSE event type and store pending approvals.

**Files:**
- Modify: `web/components/domain/agent-chat-reducer.ts` — add approval state and actions
- Modify: `web/components/domain/agent-chat-context.tsx` — handle new event, add approval response function

**Step 1: Update reducer types and state**

In `agent-chat-reducer.ts`, add approval types:

```typescript
export type ApprovalDiff = {
  hunks: Array<{
    index: number;
    old_start: number;
    old_lines: number;
    new_start: number;
    new_lines: number;
    lines: Array<{
      type: "context" | "add" | "delete";
      content: string;
      old_line_no?: number;
      new_line_no?: number;
    }>;
  }>;
  stats: { additions: number; deletions: number; hunks_count: number };
};

export type PendingApproval = {
  callId: string;
  tool: string;
  path: string;
  isNew: boolean;
  diff?: ApprovalDiff;
  content?: string; // full content for new documents
};
```

Add to `State`:
```typescript
export type State = {
  // ... existing fields ...
  pendingApproval: PendingApproval | null;
  autoApprove: boolean;
};
```

Update `initialState`:
```typescript
export const initialState: State = {
  // ... existing ...
  pendingApproval: null,
  autoApprove: false,
};
```

Add actions:
```typescript
export type Action =
  // ... existing ...
  | { type: "TOOL_APPROVAL_REQUIRED"; approval: PendingApproval }
  | { type: "TOOL_APPROVAL_RESOLVED" }
  | { type: "SET_AUTO_APPROVE"; value: boolean };
```

Add reducer cases:
```typescript
case "TOOL_APPROVAL_REQUIRED":
  return { ...state, pendingApproval: action.approval };
case "TOOL_APPROVAL_RESOLVED":
  return { ...state, pendingApproval: null };
case "SET_AUTO_APPROVE":
  return { ...state, autoApprove: action.value };
```

**Step 2: Update context to handle SSE event**

In `agent-chat-context.tsx`, update the `StreamEvent` type:

```typescript
export type StreamEvent = {
  type: "text" | "tool_start" | "tool_end" | "msg_start" | "msg_end" | "conv_id" | "error" | "tool_approval_required";
  // ... existing fields ...
  approval?: {
    callId: string;
    tool: string;
    path: string;
    isNew: boolean;
    diff?: ApprovalDiff;
    content?: string;
  };
};
```

Add the event handler in the SSE switch:
```typescript
case "tool_approval_required":
  if (event.approval) {
    dispatch({
      type: "TOOL_APPROVAL_REQUIRED",
      approval: event.approval,
    });
  }
  break;
```

**Step 3: Add `respondToApproval` function in context**

```typescript
const respondToApproval = async (
  action: "approve_all" | "approve_hunks" | "reject",
  hunkIndexes?: number[],
) => {
  const approval = stateRef.current.pendingApproval;
  if (!approval) return;
  const convId = stateRef.current.activeConversationId;
  if (!convId) return;

  try {
    await fetch("/api/agent/approval", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        conversationId: convId,
        callId: approval.callId,
        action,
        hunkIndexes: hunkIndexes ?? [],
      }),
    });
    dispatch({ type: "TOOL_APPROVAL_RESOLVED" });
  } catch (err) {
    console.error("Failed to send approval:", err);
    dispatch({
      type: "SET_ERROR",
      error: err instanceof Error ? err.message : "Failed to send approval",
    });
  }
};
```

Add to context value:
```typescript
type AgentChatContextValue = State & {
  // ... existing ...
  respondToApproval: (action: "approve_all" | "approve_hunks" | "reject", hunkIndexes?: number[]) => Promise<void>;
  setAutoApprove: (value: boolean) => void;
};
```

**Step 4: Add `autoApprove` to the chat request body**

In `sendMessage`, include autoApprove in the POST body:

```typescript
body: JSON.stringify({
  conversationId: convId || undefined,
  vaultId,
  content,
  docRefs,
  autoApprove: stateRef.current.autoApprove,
}),
```

Update `chatRequestBody` in the Go handler (`handler.go`) and Next.js route accordingly.

**Step 5: Verify build**

Run: `cd web && bun run build`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add web/components/domain/agent-chat-reducer.ts web/components/domain/agent-chat-context.tsx
git commit -m "feat(web): handle tool_approval_required SSE event in chat state"
```

---

### Task 8: Frontend — approval diff UI component

Create an inline diff component that renders in the chat when approval is pending.

**Files:**
- Create: `web/components/domain/tool-approval-card.tsx`
- Modify: `web/components/domain/agent-chat-panel.tsx` — render approval card when pending

**Step 1: Create `tool-approval-card.tsx`**

Build a component that shows the diff (reusing the `HunkView`/`LineView` pattern from `version-diff-view.tsx`) with approve/reject buttons. For new documents, show the full content as all-additions.

Key elements:
- Header showing tool name + document path
- Diff hunks with checkboxes for per-hunk approval
- Three action buttons: Approve All, Approve Selected, Reject
- For new documents: show full content in a code block with green background

```typescript
"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useAgentChat } from "./agent-chat-context";
import type { PendingApproval } from "./agent-chat-reducer";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

type Props = { approval: PendingApproval };

export function ToolApprovalCard({ approval }: Props) {
  const t = useTranslations("agent");
  const { respondToApproval } = useAgentChat();
  const [selectedHunks, setSelectedHunks] = useState<Set<number>>(new Set());
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleApproveAll = async () => {
    setIsSubmitting(true);
    await respondToApproval("approve_all");
  };

  const handleApproveSelected = async () => {
    setIsSubmitting(true);
    await respondToApproval("approve_hunks", Array.from(selectedHunks));
  };

  const handleReject = async () => {
    setIsSubmitting(true);
    await respondToApproval("reject");
  };

  const toggleHunk = (index: number) => {
    setSelectedHunks((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  // ... render diff hunks with checkboxes, action buttons ...
  // Reuse the line rendering pattern from version-diff-view.tsx
}
```

**Step 2: Integrate into `agent-chat-panel.tsx`**

Add the approval card to the streaming area of the chat panel. When `pendingApproval` is non-null, render `<ToolApprovalCard>` at the bottom of the message list (above the input).

**Step 3: Add i18n strings**

Add to `web/messages/en.json` and `web/messages/de.json` under `"agent"`:
- `approvalTitle`: "Review Changes"
- `approvalNewDoc`: "New Document"
- `approvalEditDoc`: "Edit Document"
- `approveAll`: "Approve All"
- `approveSelected`: "Approve Selected"
- `reject`: "Reject"
- `hunkCount`: "{count} changes"

**Step 4: Verify build**

Run: `cd web && bun run build`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add web/components/domain/tool-approval-card.tsx web/components/domain/agent-chat-panel.tsx web/messages/
git commit -m "feat(web): add inline diff approval card for agent write tools"
```

---

### Task 9: Frontend — auto-approve toggle

Add a toggle switch in the chat panel header to enable/disable auto-approve mode.

**Files:**
- Modify: `web/components/domain/agent-chat-panel.tsx` — add toggle
- Modify: `web/messages/en.json`, `web/messages/de.json` — add i18n strings

**Step 1: Add toggle UI**

In the chat panel header area, add a switch/toggle. Use Headless UI's `Switch` component (already available as a UI primitive):

```tsx
<div className="flex items-center gap-2">
  <Switch
    checked={autoApprove}
    onChange={(v) => setAutoApprove(v)}
    // styling...
  />
  <span className="text-xs text-slate-500">{t("autoApprove")}</span>
</div>
```

**Step 2: Add i18n strings**

- `autoApprove`: "Auto-approve writes" / "Schreibzugriffe automatisch genehmigen"

**Step 3: Verify build**

Run: `cd web && bun run build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add web/components/domain/agent-chat-panel.tsx web/messages/
git commit -m "feat(web): add auto-approve toggle for agent writes"
```

---

### Task 10: Remove the proposal system

Remove all proposal-related code now that tool approval replaces it.

**Files:**
- Delete: `internal/models/proposal.go`
- Delete: `internal/review/service.go`
- Delete: `internal/review/diff.go` (already moved to `internal/diff/`)
- Delete: `internal/review/diff_test.go` (already moved)
- Delete: `internal/db/queries_proposal.go`
- Delete: `internal/integration/proposal_test.go`
- Modify: `internal/db/schema.go` — remove `document_proposal` DDL
- Modify: `internal/graph/schema.graphqls` — remove proposal types, queries, mutations
- Modify: `internal/graph/schema.resolvers.go` — remove proposal resolvers
- Modify: `internal/graph/helpers.go` — remove proposal helpers
- Modify: `gqlgen.yml` — remove proposal model mappings (if any)
- Remove any proposal-related imports from `cmd/knowhow-server/main.go`

**Step 1: Delete files**

Remove the listed files and directories.

**Step 2: Remove DDL from schema.go**

Remove the `document_proposal` table definition, field definitions, indexes, and cascade delete event from `internal/db/schema.go`.

**Step 3: Remove from GraphQL schema**

Remove from `schema.graphqls`:
- `DocumentProposal` type
- `ProposalDiff` type
- `ProposalStatus` enum
- `ProposalSource` enum
- `ProposeDocumentUpdateInput` input
- `ApproveHunksInput` input
- `proposal` and `proposals` queries
- `proposeDocumentUpdate`, `approveProposal`, `approveProposalHunks`, `rejectProposal` mutations
- `proposals` field on `Document` type (if present)

**Step 4: Regenerate GraphQL**

Run: `just generate`

**Step 5: Remove resolver implementations**

Delete the resolver functions for removed mutations/queries from `schema.resolvers.go`. Remove proposal helper functions from `helpers.go`.

**Step 6: Update service wiring**

Remove `review.Service` creation from `cmd/knowhow-server/main.go` (or wherever it's instantiated). Remove the `reviewService` field from the GraphQL resolver struct.

**Step 7: Verify everything compiles and tests pass**

Run: `just build-all && just test`
Expected: All builds and tests pass

**Step 8: Commit**

```bash
git add -A
git commit -m "refactor: remove proposal system (replaced by agent tool approval)"
```

---

### Task 11: Integration test — agent write with approval

Write an integration test that exercises the full approval flow.

**Files:**
- Create: `internal/agent/approval_test.go`

**Step 1: Write unit test for approval registry**

```go
func TestApprovalRegistry_RegisterAndResolve(t *testing.T) {
	reg := newApprovalRegistry()
	ch := reg.register("call-1")

	go func() {
		err := reg.resolve(ApprovalResponse{
			CallID: "call-1",
			Action: ApprovalApproveAll,
		})
		require.NoError(t, err)
	}()

	resp := <-ch
	assert.Equal(t, ApprovalApproveAll, resp.Action)
}

func TestApprovalRegistry_ResolveUnknown(t *testing.T) {
	reg := newApprovalRegistry()
	err := reg.resolve(ApprovalResponse{CallID: "nope"})
	assert.Error(t, err)
}

func TestApprovalRegistry_Cancel(t *testing.T) {
	reg := newApprovalRegistry()
	ch := reg.register("call-1")
	reg.cancel()
	_, ok := <-ch
	assert.False(t, ok) // channel closed
}
```

**Step 2: Run tests**

Run: `just test`
Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/agent/approval_test.go
git commit -m "test(agent): add approval registry unit tests"
```

---

### Task 12: Update README with example prompts

**Files:**
- Modify: `README.md`

**Step 1: Add write tool examples**

Add a section documenting the new agent capabilities with example prompts:

- "Create a new guide about Docker at /guides/docker.md"
- "Update the setup guide to include the new installation step"
- "Read /guides/setup.md and add a troubleshooting section"

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add agent write tool example prompts to README"
```
