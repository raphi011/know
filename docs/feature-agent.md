# Agent Chat

LLM-powered chat agent that can search, read, create, and edit documents in your vaults. Powers the TUI chat interface and is available via the REST API.

## Overview

The agent provides a conversational interface to your knowledge base. It has access to a set of tools that let it search for information, read documents, and make changes — all within the scope of your authenticated vault. Responses stream in real-time via Server-Sent Events (SSE).

Available tools:

- **search** — hybrid search across vault documents
- **read_document** — read a document by path
- **list_labels** — list all labels in the vault
- **list_folders** — list folder structure across all vaults
- **list_folder_contents** — list contents of a specific folder
- **get_document_versions** — get version history for a document
- **list_tasks** — list tasks extracted from documents
- **create_document** — create a new document
- **edit_document** — edit an existing document
- **edit_document_section** — edit a specific section of a document
- **create_memory** — create a memory note
- **toggle_task** — toggle a task's open/done status
- **web_search** — search the web via Tavily API (requires `KNOW_TAVILY_API_KEY`)
- **fetch_youtube_transcript** — fetch YouTube video transcripts (requires `KNOW_APIFY_TOKEN`)

## How It Works

1. **User sends a message**, optionally attaching document references or local files.
2. **Agent builds a system prompt** that includes the vault's folder structure for spatial awareness.
3. **LLM decides which tools to call** based on the user's request.
4. **Tool results feed back** into the LLM until it produces a final answer.
5. **Responses stream in real-time** via SSE, so partial output appears as it is generated.

Conversation management is automatic: conversations are created on the first message, auto-titled using the LLM, and full message history is stored for context continuity.

## TUI Chat

The `know agent` command launches a terminal chat UI powered by Bubbletea v2.

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

**MCP clients:** When using know as an MCP server, tool approval is handled by the MCP client's native approval mechanism instead.

## Usage

Example prompts:

```
"Create a document at /docs/api-guide.md summarizing our API endpoints"
"Update /docs/architecture.md to include the new caching layer"
"Reorganize the docs folder — split the README into separate guides"
```

## Vault Instructions (VAULT.md)

Create a `/VAULT.md` document at the root of your vault to customize agent behavior. The content is injected into the agent's system prompt on every request — similar to how `CLAUDE.md` works for Claude Code.

### Creating VAULT.md

```
# Via agent chat
"Create /VAULT.md with the following instructions: Always respond in German."

# Via CLI import
echo "Always respond in German." > vault-instructions.md
know import vault-instructions.md /VAULT.md --vault default

# Via WebDAV, NFS, or any editor that connects to the vault
```

### Updating via the agent

Ask the agent to remember preferences and it will update `/VAULT.md`:

```
"Remember that I prefer bullet lists over prose."
"Add to VAULT.md: always cite sources with document paths."
```

### Example VAULT.md

```markdown
# Vault Instructions

- Always respond in German
- Prefer bullet lists over prose
- When summarizing documents, include the document path as a citation
- Use the "meeting-notes" template for any meeting-related requests
```

### Size limit

VAULT.md content is capped at 32 KB. Content beyond this limit is truncated with a warning.

## Reference

### REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agent/chat` | Start agent, returns 202 `{conversationId, status}` |
| `GET` | `/agent/events/{id}` | SSE stream (replay + live events, reconnectable) |
| `POST` | `/agent/cancel/{id}` | Cancel a running agent |
| `POST` | `/agent/approval` | Approve or reject a pending tool call |

### Related Docs

- [Search](feature-search.md) — how `search` works under the hood
- [MCP Server](feature-mcp.md) — using the agent tools via MCP
