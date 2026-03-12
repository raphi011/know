# Knowhow

Personal knowledge RAG database - like Obsidian / second brain but searchable, indexable, and AI-augmented.

Store any type of knowledge (people, services, concepts, documents) with flexible schemas, Markdown templates, and semantic search.

## Features

- **Flexible Entity Model**: Store anything - people, services, documents, concepts, tasks
- **Hybrid Search**: RRF fusion of BM25 full-text + vector semantic search
- **Automatic Chunking**: Long documents split into searchable chunks with context
- **Graph Relations**: Link entities with typed relationships
- **LLM Synthesis**: Ask questions and get synthesized answers from your knowledge
- **Templates**: Generate structured output (peer reviews, service summaries)
- **Multi-Provider**: Supports Google AI (Gemini), Anthropic/Voyage, OpenAI, Bedrock, Ollama for embeddings and LLM
- **Web UI**: Svelte-based document editor with CodeMirror — edit markdown documents in the browser
- **Chat Panel**: Slide-over conversational Q&A with streaming responses, multi-turn history, and document-aware search

## Installation

```bash
# Homebrew (macOS/Linux)
brew install raphi011/tap/knowhow

# Or build from source
go build -o knowhow ./cmd/knowhow
```

### Prerequisites

- **SurrealDB**: Running at `ws://localhost:8000/rpc` (default)
- An embedding provider API key (Google AI, Anthropic, OpenAI, or local Ollama)

```bash
# Start SurrealDB
surreal start --user root --pass root
```

## Quick Start

### Add Knowledge

```bash
# Add a simple note
knowhow add "SurrealDB supports HNSW indexes for vector search"

# Add a person with labels
knowhow add "John Doe is a senior SRE on the platform team" \
  --type person \
  --labels "work,team-platform"

# Add a task
knowhow add "Fix token refresh bug in auth-service" \
  --type task \
  --labels "work,auth-service,bug"

# Add with relations
knowhow add "Meeting notes: discussed auth timeout" \
  --labels "meetings" \
  --relates-to "john-doe:mentioned_in,auth-service:about"
```

### Search

```bash
# Simple search
knowhow search "authentication"

# Filter by labels
knowhow search "token refresh" --labels "work,auth-service"

# Filter by type
knowhow search "senior engineer" --type person

# Only verified knowledge
knowhow search "kubernetes" --verified
```

### Ask Questions (LLM Synthesis)

```bash
# Free-form question (streams response token by token)
knowhow ask "What do I know about John Doe?"

# Ask about a service
knowhow ask "How does the auth service work?"

# Disable streaming for scripting/piping
knowhow ask "How does auth work?" --no-stream | head -5

# Use a template for structured output (non-streaming)
knowhow ask "John Doe" --template "Peer Review" -o review.md
knowhow ask "auth-service" --template "Service Summary"

# Filter context during ask
knowhow ask "What are John's responsibilities?" --labels "work" --type person
```

**Streaming behavior:**
- Default: Streams tokens in real-time for interactive use
- Auto-disables when: writing to file (`-o`), piping output, or using templates
- Override with `--no-stream` flag

### Copy Files into a Vault

```bash
# Copy top-level files (unchanged files are automatically skipped)
knowhow cp ./docs / --vault default

# Recursive copy with labels
knowhow cp ./notes /notes --vault default -r --labels "personal"

# Dry run (preview which files would be copied)
knowhow cp ./wiki /wiki --vault default --dry-run

# Force overwrite files with different content hash
knowhow cp ./docs /docs --vault default --force
```

### Manage Relations

```bash
# Link two entities
knowhow link "john-doe" "auth-service" --type "works_on"
knowhow link "auth-service" "user-service" --type "depends_on"
```

### Update & Delete

```bash
# Update content
knowhow update "auth-service" --content "Updated documentation..."

# Add labels
knowhow update "john-doe" --labels "add:senior,promoted"

# Mark as verified
knowhow update "auth-service" --verified

# Delete (with confirmation)
knowhow delete "old-notes"

# Force delete
knowhow delete "old-notes" --force
```

### Remove Documents

```bash
# Remove a single document
knowhow rm /docs/readme.md

# Remove from a specific vault
knowhow rm /docs/readme.md --vault my-vault

# Preview what would be deleted (dry run)
knowhow rm /docs/readme.md --dry-run

# Recursively remove a folder and all its contents
knowhow rm /docs -r

# Dry run recursive delete
knowhow rm /docs -r --dry-run
```

### View Document Content

```bash
# Print a document to stdout
knowhow cat /docs/readme.md

# From a specific vault
knowhow cat /docs/readme.md --vault my-vault

# Pipe through a viewer (bat, glow, etc.)
knowhow cat /docs/readme.md --viewer "bat -l md"
KNOWHOW_VIEWER=glow knowhow cat /notes/todo.md
```

### List & Explore

```bash
# List files and folders in a vault
knowhow ls
knowhow ls /docs
knowhow ls /docs -R
knowhow ls --vault my-vault /notes

# List all entities
knowhow list

# Filter by type
knowhow list --type person

# Filter by labels
knowhow list --labels "work,banking"

# List all labels
knowhow list labels

# List all entity types
knowhow list types
```

### Vault Info

```bash
# Show vault stats (documents, chunks, embeddings, labels, members, etc.)
knowhow vault
knowhow vault default
knowhow vault my-vault --api-url http://localhost:4001
```

### Templates

```bash
# List available templates
knowhow template list

# Show template content
knowhow template show "Peer Review"

# Add custom template
knowhow template add ./my-template.md --name "My Template"

# Initialize default templates
knowhow template init
```

### Export & Backup

```bash
# Export all to Markdown files
knowhow export ./backup

# Export specific type
knowhow export ./backup --type document

# Export verified only
knowhow export ./backup --verified-only
```

### Usage Statistics

```bash
# Show server stats and token usage
knowhow usage

# Last 7 days of token usage
knowhow usage --since "7d"

# Detailed breakdown with costs
knowhow usage --detailed --costs
```

### Server Configuration

```bash
# Show server configuration (LLM, embedding, chunking settings)
knowhow config

# Output as JSON
knowhow config --json
```

## Configuration

Environment variables:

```bash
# SurrealDB
SURREALDB_URL=ws://localhost:8000/rpc
SURREALDB_NAMESPACE=knowledge
SURREALDB_DATABASE=graph
SURREALDB_USER=root
SURREALDB_PASS=root

# Embedding Provider (googleai | anthropic | openai | bedrock | ollama)
KNOWHOW_EMBED_PROVIDER=googleai
KNOWHOW_EMBED_MODEL=gemini-embedding-001
KNOWHOW_EMBED_DIMENSION=768

# LLM Provider (anthropic | openai | googleai | bedrock | ollama)
KNOWHOW_LLM_PROVIDER=anthropic
KNOWHOW_LLM_MODEL=claude-sonnet-4-20250514

# Provider API Keys
GOOGLE_AI_API_KEY=...           # For Google AI / Gemini
ANTHROPIC_API_KEY=sk-ant-...    # For Anthropic LLM + Voyage embeddings
OPENAI_API_KEY=sk-...           # For OpenAI

# SSH/SFTP Server
KNOWHOW_SSH_ENABLED=false       # Enable SSH/SFTP server
KNOWHOW_SSH_PORT=2222           # SSH listen port
KNOWHOW_SSH_HOST_KEY=           # Path to host key (auto-generates if empty)
```

## Entity Types

Suggested entity types (you can use any string):

- `person` - People (colleagues, contacts)
- `service` - Software services
- `document` - Long-form documentation
- `concept` - Ideas, technologies, patterns
- `task` - Todos, bugs, features
- `note` - Quick notes
- `project` - Projects

## Markdown Frontmatter

When ingesting Markdown files, these frontmatter fields are recognized:

```yaml
---
type: document
title: Auth Service
labels: [work, infrastructure]
summary: Handles authentication and tokens
verified: true
relates_to:
  - user-service
  - john-doe
---
```

## Web UI

The web UI provides a document editor for browsing and editing `document`-type entities, plus a slide-over chat panel for conversational Q&A against your knowledge base.

### Production

The frontend is embedded in the Go binary. Build and run:

```bash
just build
./bin/knowhow serve
# Open http://localhost:8484
```

### Development

Run the server:

```bash
just dev
# Open http://localhost:8484/login
```

### Example Prompts

```bash
# Browse all documents in the sidebar, click to open in editor
# Edit markdown content with syntax highlighting
# Save with Cmd/Ctrl+S — content saves instantly, re-indexing runs in background

# Chat panel (click the chat bubble icon in the toolbar):
# - Click "New chat" and ask: "What do I know about SurrealDB?"
# - Open a document first, then chat — search biases toward that document's labels
# - Multi-turn: ask follow-up questions, the conversation context carries over
# - Conversations persist across page reloads
```

## WebDAV

Edit documents with any WebDAV-compatible editor. Auth uses HTTP Basic Auth where the password is a knowhow API token.

```bash
# Mount a vault (macOS Finder: Go → Connect to Server)
http://localhost:8484/dav/default/

# Or use cadaver CLI
cadaver http://localhost:8484/dav/default/
# Username: anything, Password: <your API token>
```

## SSH/SFTP

Access documents via SFTP over SSH for CLI workflows, scripted uploads, and SFTP GUI clients (CyberDuck, Transmit, Filezilla). For editor integration, use WebDAV instead — it has native macOS support and no extra dependencies.

**Note:** This is an SFTP-only server (no shell access). Editor remote features like VS Code Remote SSH and Zed Remote Projects require a full shell and won't work with this server.

### Configuration

```bash
KNOWHOW_SSH_ENABLED=true       # Enable SSH server (default: false)
KNOWHOW_SSH_PORT=2222           # SSH listen port (default: 2222)
KNOWHOW_SSH_HOST_KEY=           # Path to Ed25519 host key (default: auto-generate at ~/.knowhow/host_key)
```

### Connecting

Authentication uses your knowhow API token as the SSH password:

```bash
# Connect with sftp CLI
sftp -P 2222 user@localhost
# Password: <your API token>

# List vaults
sftp> ls
default

# Browse documents
sftp> cd default
sftp> ls
notes/  projects/  meeting-notes.md

# Read/write markdown files
sftp> get notes/todo.md
sftp> put draft.md notes/draft.md
```

### Example Prompts

```bash
# "Enable SSH access for CLI document management"
KNOWHOW_SSH_ENABLED=true just dev

# "Upload my notes via SFTP"
sftp -P 2222 user@localhost <<< "put mynotes.md default/mynotes.md"

# "Batch download all docs from a vault"
sftp -P 2222 user@localhost <<< "get -r default/"
```

### Future: sshfs Mount

Mounting vaults as local directories via `sshfs` would enable opening them in any editor (VS Code, Zed, etc.). This requires [macFUSE](https://osxfuse.github.io/) and `sshfs` (`brew install macfuse sshfs`). Not yet tested with this server.

## TUI Chat

The `knowhow agent` command launches a terminal chat UI powered by Bubbletea v2.

### @-Reference File Attachments

Attach local files to your chat messages using `@` followed by a file path. The file content is sent as context to the LLM alongside your message.

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
```

```bash
# Attach an image for vision analysis
"What's shown in this image? @./screenshot.png"

# Mix text and image attachments
"Explain the architecture in the diagram using the notes @./diagram.png @./notes.md"
```

**Supported**: Text files up to 1MB (source code, markdown, config files, etc.), images up to 10MB (PNG, JPG, GIF, WebP)
**Not supported**: Binary files, extensionless filenames (use `@./Makefile` instead of `@Makefile`)

## Agent Tool Approval (Human-in-the-Loop)

When AI agents create or edit documents, changes are shown inline in the chat UI as a diff for review before being applied. This works like `git add -p` — you can approve all changes, select specific hunks, or reject entirely.

### How It Works

1. Agent calls `create_document` or `edit_document` during a conversation
2. Execution pauses and a diff card appears in the chat
3. You review the changes and choose:
   - **Approve All** — apply all changes
   - **Approve Selected** — pick individual hunks to apply (like `git add -p`)
   - **Reject** — discard changes, agent receives feedback
4. Agent continues with the result

### Auto-Approve Mode

Each conversation defaults to **review mode** (approval required). Toggle auto-approve via the shield icon in the chat header to let the agent write freely without pausing. The toggle is per-conversation.

### Example Prompts

```
# Agent creates a new document (triggers approval)
"Create a document at /docs/api-guide.md summarizing our API endpoints"

# Agent edits an existing document (triggers approval with diff)
"Update /docs/architecture.md to include the new caching layer"

# Multi-step workflow with selective approval
"Reorganize the docs folder — split the README into separate guides"
# → Each document write pauses for your review
```

### MCP Tool Approval

MCP clients (like Claude Code) use their native tool approval mechanism. When the MCP server's `create_document` or `edit_document` tools are called, the MCP client prompts for approval before execution.

## Folders

Documents are organized into first-class folders backed by DB records. Folders are auto-created when documents are added and support explicit CRUD operations.

### Example Prompts

```graphql
# Create a folder (auto-creates ancestors)
mutation {
  createFolder(vaultId: "vault-id", path: "/projects/new-idea") {
    id path name createdAt
  }
}

# List all folders in a vault
query {
  vaults { name folders { path name } }
}

# List immediate children of a folder
query {
  vaults { name folders(parentPath: "/projects") { path name } }
}

# Move a folder (moves all children and documents)
mutation {
  moveFolder(vaultId: "vault-id", oldPath: "/guides", newPath: "/docs/guides")
}

# Delete a folder (cascades to child folders and documents)
mutation {
  deleteFolder(vaultId: "vault-id", path: "/guides")
}
```

## MCP Server (AI Agent Integration)

The `knowhow mcp` command exposes your knowledge base to AI agents via the [Model Context Protocol](https://modelcontextprotocol.io/). It aggregates one or more knowhow server instances and provides 4 tools over Streamable HTTP.

### Setup

```bash
# Build
just build

# Create config
mkdir -p ~/.config/knowhow-mcp
cat > ~/.config/knowhow-mcp/config.toml << 'EOF'
port = 8585

[[instance]]
name = "private"
url = "http://localhost:8484"
token = "kh_your_token_here"
EOF

# Start
./bin/knowhow mcp
# or with a custom config path:
./bin/knowhow mcp --config /path/to/config.toml
```

### Tools

| Tool | Description |
|------|-------------|
| `search_documents` | Hybrid full-text + semantic search across instances |
| `get_document` | Retrieve a document by path with full content |
| `create_document` | Create a new document at a given path |
| `edit_document` | Edit an existing document by replacing its content |
| `edit_document_section` | Edit a specific section by heading without sending full content |
| `list_labels` | List all labels used across documents |
| `create_memory` | Create a quick memory note under `/memories/` |
| `list_folders` | Browse the folder structure of a vault |
| `list_folder_contents` | List documents and subfolders in a specific folder |
| `get_document_versions` | List version history of a document |

### Example Prompts

```
# Search across all instances
"Search my knowledge base for authentication patterns"

# Search a specific instance
"Search the work instance for onboarding docs"

# Get a specific document
"Get the document at /docs/architecture.md"

# Discover categories
"List all labels in my knowledge base"

# Browse vault structure
"Show me the folder structure of my knowledge base"

# List folder contents
"What documents are in the /docs/guides folder?"

# Create a new document
"Create a document at /docs/runbook.md with our deployment steps"

# Edit an existing document (full replacement)
"Update /docs/architecture.md to add the new caching section"

# Edit a specific section (targeted, token-efficient)
"Replace the Installation section of /docs/setup.md with the new Docker instructions"
"Add a Troubleshooting section to the end of /docs/deployment.md"
"Delete the Deprecated section from /docs/api.md"

# View document history
"Show me the version history of /docs/api-guide.md"

# Save a quick note
"Remember that the deploy pipeline requires manual approval for production"

# Create a memory with labels
"Save a note about today's meeting decisions, label it 'meetings' and 'project-x'"
```

### Multi-Instance Config

The MCP server can aggregate multiple knowhow-server instances (e.g., personal + work):

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

When no instance is specified in a tool call, all instances are searched. Specify `instance: "work"` to target a single one.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      CLI (cobra)                         │
│  add, search, ask, cp, link, update, delete, ...        │
├─────────────────────────────────────────────────────────┤
│              Service Layer                               │
│  EntityService, SearchService, IngestService             │
├─────────────────────────────────────────────────────────┤
│              Infrastructure                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │  SurrealDB  │  │ langchaingo │  │   Parser    │     │
│  │  (storage)  │  │ (LLM/embed) │  │  (chunker)  │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
```

## License

MIT
