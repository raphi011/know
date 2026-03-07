# Agent Tool Approval for Document Writes

## Problem

AI agents (internal agent, MCP) need to create and edit documents, but users want oversight over what gets written. Currently the MCP server only has `create_memory` (direct write) and the internal agent is read-only.

## Decision

Instead of a backend proposal/shadow system, use **inline tool approval** at the agent execution layer. The agent pauses when it wants to write, shows a diff, and the user approves or rejects before the write goes through. A per-conversation toggle switches between review mode (default) and auto-approve mode.

This replaces the existing `document_proposal` system entirely.

## Design

### Write Flow (Review Mode — Default)

1. Agent calls a write tool (`create_document` or `edit_document`)
2. Backend computes diff:
   - **New document**: entire content shown as additions
   - **Existing document**: hunk-based diff against current content
3. SSE event `tool_approval_required` emitted with diff payload (hunks, stats, document path)
4. Agent execution **pauses** — LLM loop waits for user response
5. Web UI renders diff inline in the chat conversation
6. User action:
   - **Approve all** — write goes through, agent gets success response
   - **Approve specific hunks** — partial content applied, agent gets success with note about which hunks were accepted
   - **Reject** — no write, agent gets rejection message and can adapt
7. Agent continues with next LLM turn

### Write Flow (Auto-Approve Mode)

1. Agent calls a write tool
2. Write goes through immediately
3. Normal `tool_end` SSE event
4. Agent continues

### MCP Flow

MCP clients (Claude Code, Cursor, etc.) have native tool approval built into the protocol. Add write tools to the MCP server:

- `create_document` — create a new document at a path
- `edit_document` — update content of an existing document

The MCP client handles showing the tool call to the user and getting approval. No custom approval logic needed on the MCP side.

### Auto-Approve Toggle

- **Scope**: per-conversation
- **Default**: review mode (approval required)
- **UI**: toggle switch in the chat header area
- **Persistence**: stored in conversation state (not persisted across page reloads — defaults back to review mode)

### SSE Event: `tool_approval_required`

```json
{
  "type": "tool_approval_required",
  "toolCallId": "tc_abc123",
  "tool": "edit_document",
  "path": "/guides/setup.md",
  "isNew": false,
  "diff": {
    "hunks": [
      {
        "oldStart": 5,
        "oldEnd": 8,
        "newStart": 5,
        "newEnd": 10,
        "lines": [
          {"type": "context", "content": "existing line"},
          {"type": "delete", "content": "old line"},
          {"type": "add", "content": "new line"},
          {"type": "add", "content": "another new line"}
        ]
      }
    ],
    "stats": {
      "additions": 2,
      "deletions": 1
    }
  }
}
```

### SSE Event: `tool_approval_response` (client → server)

```json
{
  "type": "tool_approval_response",
  "toolCallId": "tc_abc123",
  "action": "approve_all" | "approve_hunks" | "reject",
  "hunkIndexes": [0, 2],
  "notes": "optional user note"
}
```

### Agent Tool Definitions

**`create_document`**:
- Input: `path`, `content`, `labels` (optional)
- Creates a new document at the given path
- Fails if document already exists at that path

**`edit_document`**:
- Input: `path`, `content`
- Replaces the full content of an existing document
- Fails if document doesn't exist

### Pause/Resume Mechanism

The agent's LLM loop (in `internal/agent/service.go`) currently processes tool calls synchronously. For review mode:

1. When a write tool is called, instead of executing immediately, emit `tool_approval_required`
2. Block on a channel (`chan ApprovalResponse`) waiting for the user's response
3. The HTTP/SSE handler receives the approval response and sends it on the channel
4. Tool execution resumes with the approval decision
5. If approved: execute write, return success to LLM
6. If rejected: return rejection message to LLM (agent can retry or move on)

### Partial Hunk Approval

When the user approves specific hunks:

1. Compute hunks from diff (current content vs proposed content)
2. Apply only selected hunks to produce merged content
3. Write merged content via document service
4. Return success to agent with a note: "Changes partially applied (hunks 1, 3 of 4 approved)"

The agent sees a success response and continues. It doesn't know some hunks were rejected — it just knows the write "worked."

## What Changes

| Area | Change |
|---|---|
| `internal/agent/service.go` | Add `create_document` and `edit_document` tools; add pause/resume on approval channel |
| `internal/agent/service.go` | New SSE event types: `tool_approval_required` |
| `internal/agent/approval.go` (new) | Approval channel management, diff computation for tool calls |
| `cmd/knowhow-mcp/tools.go` | Add `create_document` and `edit_document` MCP tools (direct writes, MCP client handles approval) |
| `web/` | Inline diff renderer component for chat |
| `web/` | Approve all / approve hunks / reject buttons |
| `web/` | Auto-approve toggle in chat header |
| `web/` | SSE handler for `tool_approval_required` events + response endpoint |

## What Gets Removed

- `internal/models/proposal.go` — proposal model
- `internal/review/` — entire review service package
- `internal/db/queries_proposal.go` — proposal DB queries
- `document_proposal` SurrealDB table + DDL
- GraphQL proposal mutations/queries (`proposeDocumentUpdate`, `approveProposal`, etc.)
- Related integration tests

## Rollback Safety

The existing versioning system provides rollback. Every approved write creates a new document version. Users can view version history and restore any previous version from the web UI.

## Open Questions

None — design approved.
