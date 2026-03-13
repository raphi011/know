# MCP Server

The `knowhow mcp` command exposes your knowledge base to AI agents via the Model Context Protocol. It aggregates one or more knowhow server instances and provides tools over Streamable HTTP, authenticated with Bearer tokens.

## How It Works

The MCP server acts as an aggregation layer in front of one or more knowhow instances. Each instance is a separate knowhow server with its own vault, token, and URL. When a tool call does not specify an instance, all configured instances are searched. To target a single instance, pass `instance: "work"` (or whichever name) in the tool call.

Communication uses Streamable HTTP (not stdio), so any MCP-compatible client can connect over the network.

## Setup & Configuration

### Build

```bash
just build
```

### Configuration file

Create a config file at `~/.config/knowhow-mcp/config.toml`:

```toml
port = 8585

[[instance]]
name = "private"
url = "http://localhost:8484"
token = "kh_private_token"

[[instance]]
name = "work"
url = "http://work-server:8484"
token = "kh_work_token"
```

Each `[[instance]]` entry defines a knowhow server to aggregate:

| Field   | Description                          |
|---------|--------------------------------------|
| `name`  | Identifier used in tool calls        |
| `url`   | Base URL of the knowhow server       |
| `token` | Bearer token for authentication      |

### Start the server

```bash
# Default config location
./bin/knowhow mcp

# Custom config path
./bin/knowhow mcp --config /path/to/config.toml
```

### Client setup

#### Claude Code

Add to `.claude/settings.json`:

```json
{
  "mcpServers": {
    "knowhow": {
      "url": "http://localhost:8585/mcp"
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
      "url": "http://localhost:8585/mcp"
    }
  }
}
```

## Usage

### Searching and reading

- "Search my knowledge base for authentication patterns"
- "Search the work instance for onboarding docs"
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
| `search_documents` | Hybrid BM25 + vector search across instances. Supports `label`, `doc_type`, and `folder` filters. |
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
