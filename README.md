# Know

Personal knowledge RAG database - like Obsidian / second brain but searchable, indexable, and AI-augmented.

Store any type of knowledge (people, services, concepts, documents) with flexible schemas, Markdown templates, and semantic search.

## Features

- **[Hybrid Search](docs/feature-search.md)** — RRF fusion of BM25 full-text + vector semantic search, with LLM synthesis
- **[Document Ingestion](docs/feature-ingestion.md)** — Markdown pipeline: parse, embed, link, chunk. Wiki-links, frontmatter, versioning
- **[MCP Server](docs/feature-mcp.md)** — Expose your knowledge base to AI agents via Model Context Protocol
- **[Agent Chat](docs/feature-agent.md)** — Terminal and web chat UI with tool calling, approval flow, and SSE streaming
- **[Memory System](docs/feature-memory.md)** — Project-scoped memories with decay scoring, auto-archiving, and consolidation
- **[Vaults & Access Control](docs/feature-vaults.md)** — Folders, roles, share links, multi-user collaboration
- **[WebDAV](docs/feature-webdav.md)** — Edit documents with any WebDAV-compatible editor
- **[Remotes](docs/feature-remotes.md)** — Multi-server federation with namespace routing
- **Multi-Provider** — Supports Google AI, Anthropic/Voyage, OpenAI, Bedrock, Ollama for embeddings and LLM

## Installation

```bash
# Homebrew (macOS/Linux)
brew install raphi011/tap/know

# Or build from source
go build -o know ./cmd/know
```

### Prerequisites

- **SurrealDB** running at `ws://localhost:8000/rpc` (default)
- An embedding provider API key (Google AI, Anthropic, OpenAI, or local Ollama)

## Quick Start

```bash
# 1. Start SurrealDB
surreal start --user root --pass root

# 2. Bootstrap (creates user, vault, token)
know db seed

# 3. Start the server
know serve

# 4. Copy documents into a vault
know cp ./docs / --vault default

# 5. Launch the agent chat TUI
know agent
```

## Configuration

Essential environment variables:

```bash
# SurrealDB
SURREALDB_URL=ws://localhost:8000/rpc
SURREALDB_USER=root
SURREALDB_PASS=root

# Embedding Provider (googleai | anthropic | openai | bedrock | ollama)
KNOW_EMBED_PROVIDER=googleai
KNOW_EMBED_MODEL=gemini-embedding-001

# LLM Provider (anthropic | openai | googleai | bedrock | ollama)
KNOW_LLM_PROVIDER=anthropic
KNOW_LLM_MODEL=claude-sonnet-4-20250514

# Provider API Keys
GOOGLE_AI_API_KEY=...
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
```

See individual [feature docs](docs/) for feature-specific configuration.

## Architecture

```
+---------------------------------------------------------+
|                      CLI (cobra)                        |
|  cp, agent, remote, vault, ls, cat, ...                  |
+---------------------------------------------------------+
|              Service Layer                              |
|  DocumentService, SearchService, AgentService           |
+---------------------------------------------------------+
|              Infrastructure                             |
|  +-------------+  +-------------+  +-------------+     |
|  |  SurrealDB  |  |  LLM/Embed  |  |   Parser    |     |
|  |  (storage)  |  |  (providers)|  |  (chunker)  |     |
|  +-------------+  +-------------+  +-------------+     |
+---------------------------------------------------------+
```

## Documentation

### Feature Guides

- [MCP Server](docs/feature-mcp.md) — AI agent integration via Model Context Protocol
- [Agent Chat](docs/feature-agent.md) — TUI and web chat with tool approval
- [Search](docs/feature-search.md) — Hybrid search and LLM synthesis
- [Document Ingestion](docs/feature-ingestion.md) — Pipeline, frontmatter, wiki-links, versioning
- [Memory System](docs/feature-memory.md) — Decay scoring, consolidation, archiving
- [Vaults & Access Control](docs/feature-vaults.md) — Folders, roles, share links
- [WebDAV](docs/feature-webdav.md) — Edit documents with any editor
- [Remotes](docs/feature-remotes.md) — Multi-server federation

### Project Docs

- [Teleport Proxy](docs/teleport-proxy.md) — Bedrock integration via Teleport AWS proxy

## License

MIT
