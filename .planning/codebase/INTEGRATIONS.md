# External Integrations

**Analysis Date:** 2026-02-01

## APIs & External Services

**Hugging Face Model Hub:**
- Service: Hugging Face transformer models for NLP
  - `sentence-transformers` SDK loads models from hub
  - Embedding model: `all-MiniLM-L6-v2` - lazy-loaded at first embedding request
  - NLI model: `cross-encoder/nli-deberta-v3-base` - lazy-loaded for contradiction detection
  - Auth: No auth required; models cached locally after first download
  - Location: `memcp/utils/embedding.py` - `_get_embedder()`, `_get_nli_model()`

**Model Context Protocol (MCP):**
- Service: Communication protocol for AI agents
  - Implementation: `FastMCP` from fastmcp 2.14.0+
  - Tools exposed to Claude and other MCP clients via stdio
  - No external API calls; protocol is local/stdio-based
  - Location: `memcp/server.py` - main server entry point

## Data Storage

**Databases:**
- SurrealDB v3.0.0+
  - Connection: WebSocket via `SURREALDB_URL` (default: `ws://localhost:8000/rpc`)
  - Client: `surrealdb` Python SDK (AsyncSurreal)
  - Authentication:
    - Root level: Username/password via `SURREALDB_USER`, `SURREALDB_PASS`
    - Database level: Namespace + database-scoped credentials
    - Auth level controlled by `SURREALDB_AUTH_LEVEL` env var
  - Location: `memcp/db.py` - connection, schema, all query functions
  - Schema: See `SCHEMA_SQL` in `memcp/db.py` (lines 161-260)
    - Tables: `entity`, `episode`, `procedure`, `relates`, `extracted_from`
    - Indexes: HNSW vector index (384 dimensions), BM25 fulltext, unique constraints
    - Namespace: `knowledge` (configurable), Database: `graph` (configurable)

**File Storage:**
- Local filesystem only
  - Log file: `MEMCP_LOG_FILE` (default: `/tmp/memcp.log`)
  - No remote file storage integration

**Caching:**
- In-memory model caching (sentence-transformers models after first load)
  - Thread-safe lazy loading with locking
  - Location: `memcp/utils/embedding.py` - `_embedder`, `_nli_model` globals

## Authentication & Identity

**Auth Provider:**
- Custom SurrealDB authentication
  - Implementation: Direct SurrealDB user/password via SDK
  - Root-level auth: Authenticates as database admin
  - Database-level auth: Scoped credentials per database
  - MCP tools: No client authentication (stdio-based, single agent per connection)
  - Location: `memcp/db.py` lines 286-313 - `app_lifespan()` authentication

**Authorization:**
- No role-based access control; SurrealDB auth handles all access
- Multi-tenancy via context/namespace field: `MEMCP_DEFAULT_CONTEXT` env var
  - All entities and episodes can be filtered by `context` field for project isolation
  - Location: `memcp/db.py` - context detection and filtering functions

## Monitoring & Observability

**Error Tracking:**
- None (no Sentry, Rollbar, etc.)
- Errors logged to file and stderr via Python logging
- Location: `memcp/server.py` lines 7-19 - logging configuration

**Logs:**
- Python logging with FileHandler and StreamHandler
  - File: `MEMCP_LOG_FILE` (default: `/tmp/memcp.log`)
  - Console: stderr (captured by Docker logs)
  - Level: `MEMCP_LOG_LEVEL` env var (default: INFO)
- Application logs identify module: `memcp.db`, `memcp.server`, `memcp.webui`, etc.
- SurrealDB connection diagnostics: stderr messages during app_lifespan startup/failure

**Metrics:**
- Query performance not explicitly tracked
- Access patterns tracked in database: `access_count`, `accessed` timestamp on entities/episodes
- No APM integration (Datadog, New Relic, etc.)

## CI/CD & Deployment

**Hosting:**
- Docker containers (Docker Compose for local dev)
- Kubernetes with Helm chart (`helm/memcp/Chart.yaml`)
- Multi-stage Dockerfile: frontend builder (Node) + application runtime (Python)

**CI Pipeline:**
- Not detected in codebase; no GitHub Actions, GitLab CI, or Jenkins config files
- Manual testing required before deployment
- Commands: `uv run pytest test_*.py` for unit/integration tests

**Deployment:**
- Docker Compose: `docker-compose.yml` - SurrealDB + application stack
- Kubernetes: Helm templates in `helm/memcp/templates/`
  - Deployment manifest: `deployment.yaml`
  - Service: `service.yaml` (ClusterIP)
  - Ingress: `ingress.yaml` (for external access)
  - Secret: `secret.yaml` (SurrealDB credentials)
  - ServiceAccount: `serviceaccount.yaml`

## Environment Configuration

**Required Environment Variables:**

Production-critical:
- `SURREALDB_URL` - SurrealDB connection endpoint
- `SURREALDB_USER`, `SURREALDB_PASS` - Database credentials
- `SURREALDB_NAMESPACE`, `SURREALDB_DATABASE` - Scope settings

Optional but important:
- `SURREALDB_AUTH_LEVEL` - Set to `database` for tighter scoping
- `MEMCP_DEFAULT_CONTEXT` - Pre-filter all queries to a project namespace
- `MEMCP_LOG_LEVEL` - DEBUG for development, INFO for production

**Secrets Location:**
- Kubernetes: `helm/memcp/templates/secret.yaml` - templated secrets
- Docker Compose: Hardcoded in `docker-compose.yml` (development only: user=root, pass=root)
- Environment: Passed via `.env` file or environment variables (never committed)
- 1Password/vault integration: Not detected; manual credential management

## Webhooks & Callbacks

**Incoming Webhooks:**
- None detected
- MCP tools are called by Claude/agents directly, not via HTTP webhooks

**Outgoing Webhooks:**
- None detected
- No integrations with external services that require callbacks

## Third-Party SDKs & Libraries

**AI/ML:**
- `sentence-transformers` - Hugging Face transformer library for embeddings (384-dim)
- Cross-encoder NLI model for contradiction detection

**Protocol/Communication:**
- `fastmcp` - Model Context Protocol implementation
- `strawberry-graphql` - GraphQL schema framework (optional, for webui)

**Web Frameworks:**
- `fastapi` - REST API server (optional, for webui)
- `uvicorn` - ASGI server
- `next` - React SSR framework
- `recharts` - Charting library for dashboard

**Database:**
- `surrealdb` - Python async client for SurrealDB

**Data & Validation:**
- `pydantic` - Python data validation and schema

**Process Management:**
- `supervisor` - Multi-process manager in Docker

## Known Integration Limitations

**Claude Code MCP Sampling (Not Supported):**
- Features removed due to lack of MCP sampling support in Claude Code:
  - `memorize_file` tool - was using LLM to extract entities from documents
  - `auto_tag` parameter in `remember` - was using LLM to generate labels
  - `summarize` parameter in `search` - was using LLM to summarize results
- Alternative: Call Anthropic API directly with API key instead of MCP sampling

**SurrealDB v3.0 Limitations:**
- CBOR tag 56 (set type) not supported by Python SDK
  - Workaround: Use `array<T>` instead of `set<T>` throughout schema (see TODOs in `memcp/db.py`)
- Subquery syntax changes in v3.0: `.field` syntax deprecated
  - Workaround: Python-side list processing instead of subqueries

**Document Parsing:**
- No PDF, DOCX, PPTX support currently
- Plain text/markdown only
- Future: `docling` library could add multi-format document support

---

*Integration audit: 2026-02-01*
