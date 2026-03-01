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
# Build from source
go build -o knowhow ./cmd/knowhow

# Install to path
go install ./cmd/knowhow
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

### Ingest Markdown Files

```bash
# Scrape a directory (unchanged files are automatically skipped)
knowhow scrape ./docs

# With labels
knowhow scrape ./notes --labels "personal"

# Extract entity relations using LLM
knowhow scrape ./specs --extract-graph

# Dry run (preview which files would be ingested)
knowhow scrape ./wiki --dry-run

# Force re-ingest all files (skip change detection)
knowhow scrape ./docs --force
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

### List & Explore

```bash
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

# Web frontend (stateless — server connections configured at login)
SESSION_SECRET=your-secret-here   # Encrypts session cookies
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
just build-server
./bin/knowhow-server
# Open http://localhost:8484
```

### Development

Run the Go server and Vite dev server side by side:

```bash
# Terminal 1: Go API server
just dev

# Terminal 2: Vite dev server with hot reload
just web-dev
# Open http://localhost:5173
```

The Vite dev server proxies `/query` requests to the Go server on port 8484.

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

## Document Proposals (Agent Approval System)

AI agents can propose updates to existing documents, with a human-in-the-loop review system for approving or rejecting individual diff hunks.

### Example Prompts

```graphql
# Agent proposes an update to a document
mutation {
  proposeDocumentUpdate(input: {
    vaultId: "vault-id"
    path: "docs/architecture.md"
    proposedContent: "# Architecture\n\nUpdated content..."
    description: "Added caching layer section"
    source: AI_SUGGESTED
  }) {
    id
    status
    diff {
      stats { additions deletions hunksCount }
      hunks { index header lines { type content } }
    }
  }
}

# List pending proposals
query {
  proposals(vaultId: "vault-id", status: PENDING) {
    id
    document { path title }
    description
    hasConflict
    diff { stats { additions deletions } }
  }
}

# Approve entire proposal
mutation {
  approveProposal(id: "proposal-id", notes: "Looks good") {
    id path content
  }
}

# Approve specific hunks (like git add -p)
mutation {
  approveProposalHunks(input: {
    proposalId: "proposal-id"
    hunkIndexes: [0, 2]
    notes: "Accepted intro changes, skipped conclusion"
  }) {
    id path content
  }
}

# Reject a proposal
mutation {
  rejectProposal(id: "proposal-id", notes: "Needs more detail")
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      CLI (cobra)                         │
│  add, search, ask, scrape, link, update, delete, ...    │
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
