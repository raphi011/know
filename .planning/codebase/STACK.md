# Technology Stack

**Analysis Date:** 2026-02-01

## Languages

**Primary:**
- Python 3.11+ - MCP server, database layer, GraphQL backend
- TypeScript 5.7 - Next.js dashboard frontend
- Node.js 22 (Alpine) - Frontend runtime in Docker
- SurrealQL - Database query language for SurrealDB

**Secondary:**
- YAML - Kubernetes/Helm configuration and Docker Compose
- Shell - Docker entrypoints and supervisord configuration

## Runtime

**Environment:**
- Python 3.11-slim (Docker base image)
- Node.js 22-alpine (Docker frontend builder)
- uvicorn 0.32.0+ - ASGI application server for FastAPI

**Package Manager:**
- uv (Astral) - Python package manager and installer
- npm - Node.js package management
- Lockfiles: `uv.lock` (Python), `package-lock.json` (Node.js)

## Frameworks

**Core Backend:**
- FastMCP 2.14.0+ - MCP (Model Context Protocol) server framework
- Pydantic 2.0.0+ - Data validation and serialization for Python models
- FastAPI 0.115.0 - REST API framework for GraphQL backend (optional, webui extra)

**Frontend:**
- Next.js 15.1.0 - React framework with server-side rendering
- React 19.0.0 - UI component library
- TypeScript 5.7.0 - Type-safe JavaScript
- Tailwind CSS 3.4.17 - Utility-first CSS framework
- PostCSS 8.4.49 - CSS processing (required for Tailwind)
- Autoprefixer 10.4.20 - CSS vendor prefixing
- Recharts 2.15.0 - React charting library for visualizations

**GraphQL:**
- Strawberry GraphQL 0.252.0 - Python GraphQL framework (optional, webui extra)
- Built on top of FastAPI with `@strawberry.type` decorators

**Testing:**
- pytest 9.0.2 - Python test runner
- pytest-asyncio 1.3.0 - Async test support
- pytest-timeout 2.4.0 - Test timeout enforcement
- Markers: `integration` (requires SurrealDB), `embedding` (slow model loading)

**Build & Development:**
- hatchling - Python build backend for setuptools
- next (build command) - Next.js compiler and bundler

## Key Dependencies

**Critical:**
- surrealdb 0.3.0 - Python SDK for SurrealDB async connection and queries
- sentence-transformers 2.2.0 - Hugging Face transformers for embeddings (NLP models)
  - Model: `all-MiniLM-L6-v2` (384-dimensional embeddings, lazy-loaded)
  - Model: `cross-encoder/nli-deberta-v3-base` (for NLI/contradiction detection, lazy-loaded)

**Infrastructure:**
- uvicorn[standard] 0.32.0 - Full ASGI server with uvloop, httptools
- strawberry-graphql[fastapi] 0.252.0 - GraphQL schema and FastAPI integration

**Deployment:**
- supervisor - Process manager for running multiple services in Docker
- @types/node 22.10.0 - TypeScript definitions for Node.js
- @types/react 19.0.0 - TypeScript definitions for React
- @types/react-dom 19.0.0 - TypeScript definitions for React DOM

## Configuration

**Environment Variables:**

SurrealDB Connection:
- `SURREALDB_URL` - WebSocket URL (default: `ws://localhost:8000/rpc`)
- `SURREALDB_NAMESPACE` - Database namespace (default: `knowledge`)
- `SURREALDB_DATABASE` - Database name (default: `graph`)
- `SURREALDB_USER` - Auth username (default: `root`)
- `SURREALDB_PASS` - Auth password (default: `root`)
- `SURREALDB_AUTH_LEVEL` - Auth scope: `root` or `database` (default: `root`)

Memory Features:
- `MEMCP_QUERY_TIMEOUT` - Query timeout in seconds (default: `30`)
- `MEMCP_DEFAULT_CONTEXT` - Default project context namespace (default: none)
- `MEMCP_CONTEXT_FROM_CWD` - Auto-detect context from git origin or cwd (default: `false`)
- `MEMCP_ALLOW_CUSTOM_TYPES` - Allow entity types beyond predefined ontology (default: `true`)

Server:
- `MEMCP_LOG_FILE` - Log file path (default: `/tmp/memcp.log`)
- `MEMCP_LOG_LEVEL` - Log verbosity: DEBUG, INFO, WARNING, ERROR (default: `INFO`)
- `HOSTNAME` - Server bind address (default: `0.0.0.0`)
- `PORT` - Next.js server port (default: `3000`)

**Build Configuration:**
- `pyproject.toml` - Python project metadata, dependencies, test config
  - Test configuration: asyncio_mode=auto, timeout=30s with thread method
  - Optional extras: `[webui]` for FastAPI + GraphQL, `[dev]` for testing
- `package.json` - Node.js project config (Next.js, React, build scripts)
- `Dockerfile` - Multi-stage build: frontend (Node) → backend (Python)
- `docker-compose.yml` - Local dev: SurrealDB + containerized application
- `supervisord.conf` - Process manager config for Docker runtime
- `.env*` patterns not committed; environment vars passed at runtime

## Platform Requirements

**Development:**
- Python 3.11 or later
- Node.js 22 or later
- uv package manager (Astral)
- Docker & Docker Compose (for SurrealDB local dev)
- SurrealDB instance running (default: ws://localhost:8000/rpc)

**Production:**
- Docker container (Python 3.11-slim base)
- Docker Compose or Kubernetes (Helm chart available)
- Deployed to cloud platform or self-hosted (see `helm/memcp/` for Kubernetes)
- SurrealDB instance (managed separately or in cluster)
- HTTP(S) load balancer or ingress for port 3000 (Next.js) and 8080 (GraphQL API)

**Containerization:**
- Dockerfile uses two-stage build for efficiency
- Frontend: Node 22-alpine builder stage
- Backend: Python 3.11-slim final stage
- Supervisor manages two processes: uvicorn (GraphQL on 8080) and Node.js (Next.js on 3000)
- Exposed ports: 3000 (Next.js), 8080 (GraphQL API)

---

*Stack analysis: 2026-02-01*
