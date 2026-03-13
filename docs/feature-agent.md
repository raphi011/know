# Agent Chat

LLM-powered chat agent that can search, read, create, and edit documents in your vaults. Powers the TUI chat interface and is available via the REST API.

## Overview

The agent provides a conversational interface to your knowledge base. It has access to a set of tools that let it search for information, read documents, and make changes — all within the scope of your authenticated vault. Responses stream in real-time via Server-Sent Events (SSE).

Available tools:

- **kb_search** — hybrid search across vault documents
- **read_document** — read a document by path
- **list_labels** — list all labels in the vault
- **list_folders** — list top-level folder structure
- **list_folder_contents** — list contents of a specific folder
- **create_document** — create a new document
- **edit_document** — edit an existing document
- **edit_document_section** — edit a specific section of a document
- **web_search** — search the web via Tavily API (only after explicit user permission)

## How It Works

1. **User sends a message**, optionally attaching document references or local files.
2. **Agent builds a system prompt** that includes the vault's folder structure for spatial awareness.
3. **LLM decides which tools to call** based on the user's request.
4. **Tool results feed back** into the LLM until it produces a final answer.
5. **Responses stream in real-time** via SSE, so partial output appears as it is generated.

Conversation management is automatic: conversations are created on the first message, auto-titled using the LLM, and full message history is stored for context continuity.

## TUI Chat

The `knowhow agent` command launches a terminal chat UI powered by Bubbletea v2.

### File Attachments

Attach local files to chat messages using `@` followed by a file path:

```bash
# Attach a file with a relative path
"Explain this code @./internal/tui/app.go"

# Attach multiple files
"Compare @./old.go and @./new.go"

# Absolute and home-relative paths
"Review @/etc/nginx/nginx.conf"
"Check @~/Documents/notes.md"

# Bare filename (must contain a dot)
"What does @main.go do?"

# Attach an image for vision analysis
"What's shown in this image? @./screenshot.png"
```

**Supported:** Text files up to 1MB, images up to 10MB (PNG, JPG, GIF, WebP).

**Not supported:** Binary files, extensionless filenames.

## Tool Approval

When the agent creates or edits documents, changes are shown inline as a diff for review before being applied. The flow works like `git add -p`:

1. Agent calls `create_document` or `edit_document`.
2. Execution pauses and a diff card appears in the TUI.
3. User reviews the changes and chooses an action:
   - **Approve All** — accept the entire change
   - **Approve Selected** — accept individual hunks
   - **Reject** — discard the change
4. Agent continues with the approval result.

**Auto-Approve Mode:** A per-conversation toggle (shield icon in the TUI) skips the approval step and applies all changes immediately.

**MCP clients:** When using knowhow as an MCP server, tool approval is handled by the MCP client's native approval mechanism instead.

## Usage

Example prompts:

```
"Create a document at /docs/api-guide.md summarizing our API endpoints"
"Update /docs/architecture.md to include the new caching layer"
"Reorganize the docs folder — split the README into separate guides"
```

## Reference

### REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agent/chat` | Send a message and receive an SSE event stream |
| `POST` | `/agent/approval` | Approve or reject a pending tool call |

### Related Docs

- [Search](feature-search.md) — how `kb_search` works under the hood
- [MCP Server](feature-mcp.md) — using the agent tools via MCP
- [SSE](sse.md) — event streaming architecture
