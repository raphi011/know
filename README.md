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
- **Multi-Provider**: Supports Ollama (local), OpenAI, Anthropic for embeddings and LLM

## Installation

```bash
# Build from source
go build -o knowhow ./cmd/knowhow

# Install to path
go install ./cmd/knowhow
```

### Prerequisites

- **SurrealDB**: Running at `ws://localhost:8000/rpc` (default)
- **Ollama** (optional): For local embeddings and LLM

```bash
# Start SurrealDB
surreal start --user root --pass root

# Pull embedding model (if using Ollama)
ollama pull all-minilm:l6-v2
ollama pull llama3.2
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
# Scrape a directory
knowhow scrape ./docs

# With labels
knowhow scrape ./notes --labels "personal"

# Extract entity relations using LLM
knowhow scrape ./specs --extract-graph

# Dry run (preview)
knowhow scrape ./wiki --dry-run
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

# Embedding Provider (ollama | openai | anthropic)
KNOWHOW_EMBED_PROVIDER=ollama
KNOWHOW_EMBED_MODEL=all-minilm:l6-v2
KNOWHOW_EMBED_DIMENSION=384

# LLM Provider (ollama | openai | anthropic)
KNOWHOW_LLM_PROVIDER=ollama
KNOWHOW_LLM_MODEL=llama3.2

# Provider API Keys (if using cloud providers)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# Ollama host (if using ollama)
OLLAMA_HOST=http://localhost:11434
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
