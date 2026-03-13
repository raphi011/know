# MCP Server

Knowhow exposes your knowledge base to AI agents via the Model Context Protocol. The MCP endpoint is embedded in the main `knowhow serve` process at `/mcp`, authenticated with Bearer tokens over Streamable HTTP.

## How It Works

The MCP endpoint is part of the main knowhow server — no separate process or config file needed. It shares the same authentication, database connection, and vault scoping as the REST API. Communication uses Streamable HTTP (not stdio), so any MCP-compatible client can connect over the network.

For multi-server setups (e.g. separate work and personal instances), use the [Remotes](feature-remotes.md) feature to federate across servers.

## Setup

### Start the server

```bash
knowhow serve
# MCP endpoint available at http://localhost:8484/mcp
```

### Client setup

#### Claude Code

Add to `.claude/settings.json`:

```json
{
  "mcpServers": {
    "knowhow": {
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
    "knowhow": {
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

### Write tools

| Tool | Description |
|------|-------------|
| `create_document` | Create a new document at a given path. Fails if the document already exists. |
| `edit_document` | Replace full document content with optimistic concurrency via `expected_hash`. |
| `edit_document_section` | Edit a specific section by heading without sending the full document content. |
| `create_memory` | Create a project-scoped memory with decay scoring. Auto-adds date prefix and memory label. |
| `retrieve_memories` | Retrieve project memories sorted by relevance, with auto-archive and consolidation. |
| `delete_memory` | Delete a specific project memory. |
