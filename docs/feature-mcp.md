# MCP Server

Know exposes your knowledge base to AI agents via the Model Context Protocol. The MCP endpoint is embedded in the main `know serve` process at `/mcp`, authenticated with Bearer tokens over Streamable HTTP.

## How It Works

The MCP endpoint is part of the main know server — no separate process or config file needed. It shares the same authentication, database connection, and vault scoping as the REST API. Communication uses Streamable HTTP (not stdio), so any MCP-compatible client can connect over the network.

For multi-server setups (e.g. separate work and personal instances), use the [Remotes](feature-remotes.md) feature to federate across servers.

## Setup

### Start the server

```bash
know serve
# MCP endpoint available at http://localhost:8484/mcp
```

### Native Authentication (OAuth)

If the server has OIDC enabled, Claude Code can authenticate directly via browser login:

```bash
# Add with OAuth support
claude mcp add --transport http --client-id know-mcp know http://localhost:8484/mcp

# Then authenticate via /mcp in Claude Code
```

This uses the OAuth 2.0 Authorization Code + PKCE flow. The server proxies authentication to the configured OIDC provider and issues a `kh_` API token that Claude Code stores in the system keychain.

For servers without OIDC, use bearer token authentication:
```bash
claude mcp add --transport http know http://localhost:8484/mcp \
  --header "Authorization: Bearer ${KNOW_TOKEN}"
```

### Client setup

#### Claude Code

Add to `.claude/settings.json`:

```json
{
  "mcpServers": {
    "know": {
      "url": "http://localhost:8484/mcp",
      "headers": {
        "Authorization": "Bearer kh_your_token"
      }
    }
  }
}
```

#### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "know": {
      "url": "http://localhost:8484/mcp",
      "headers": {
        "Authorization": "Bearer kh_your_token"
      }
    }
  }
}
```

## Usage

### Searching and reading

- "Search my knowledge base for authentication patterns"
- "Search for onboarding docs in the work vault"
- "Get the document at /docs/architecture.md"
- "List all labels in my knowledge base"
- "Show me the folder structure of my knowledge base"
- "What documents are in the /docs/guides folder?"
- "Show me the version history of /docs/api-guide.md"

### Creating and editing documents

- "Create a document at /docs/runbook.md with our deployment steps"
- "Update /docs/architecture.md to add the new caching section"
- "Replace the Installation section of /docs/setup.md with the new Docker instructions"

### Tasks

- "Show me all my open tasks"
- "What tasks are due this week?"
- "List open tasks in my daily notes folder"
- "Show me tasks labeled #work"
- "Mark the PR review task as done"
- "Complete the deploy staging task"

### Memories

- "Remember that the deploy pipeline requires manual approval for production"
- "Load my project memories for this repository"

## Reference

### Read-only tools

| Tool | Description |
|------|-------------|
| `search_documents` | Hybrid BM25 + vector search across vault documents. Supports `label`, `doc_type`, and `folder` filters. |
| `get_document` | Retrieve a document by path with full content, metadata, content hash, and optional section outline. |
| `list_labels` | List all labels used across documents (cached 60s). |
| `list_folders` | Browse the folder structure of a vault (cached 60s). |
| `list_folder_contents` | List documents and subfolders in a specific folder. |
| `get_document_versions` | List version history of a document. |
| `list_tasks` | List tasks (markdown checkboxes) extracted from documents, with filters for status, labels, due dates, and folders. |

### Write tools

| Tool | Description |
|------|-------------|
| `create_document` | Create a new document at a given path. Fails if the document already exists. |
| `edit_document` | Replace full document content with optimistic concurrency via `expected_hash`. |
| `edit_document_section` | Edit a specific section by heading without sending the full document content. |
| `create_memory` | Create a project-scoped memory with decay scoring. Auto-adds date prefix and memory label. |
| `retrieve_memories` | Retrieve project memories sorted by relevance, with auto-archive and consolidation. |
| `delete_memory` | Delete a specific project memory. |
| `toggle_task` | Toggle a task's checkbox status (open ↔ done) in the source markdown document. |
