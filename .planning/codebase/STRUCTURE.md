# Codebase Structure

**Analysis Date:** 2026-02-01

## Directory Layout

```
/Users/raphaelgruber/Git/memcp/migrate-to-go/
├── memcp/                       # Main Python MCP server package
│   ├── __init__.py             # Entry point (lazy loads server.py)
│   ├── server.py               # Main FastMCP app, resource/prompt definitions
│   ├── models.py               # Pydantic response models
│   ├── db.py                   # SurrealDB layer, all query functions
│   ├── servers/                # Sub-servers by feature domain
│   │   ├── __init__.py         # Exports all sub-servers
│   │   ├── search.py           # Search, get_entity, list_labels, list_types
│   │   ├── persist.py          # Remember, forget
│   │   ├── episode.py          # add_episode, search_episodes, get_episode, delete_episode
│   │   ├── procedure.py        # create_procedure, search_procedures, get_procedure, delete_procedure
│   │   ├── graph.py            # traverse, find_path
│   │   └── maintenance.py      # reflect, check_contradictions
│   ├── api/                    # Optional FastAPI + GraphQL web interface
│   │   ├── __init__.py
│   │   ├── main.py             # FastAPI app with lifespan, GraphQL router
│   │   └── schema.py           # Strawberry GraphQL type definitions
│   ├── utils/                  # Cross-cutting utilities
│   │   ├── __init__.py         # Exports embed, check_contradiction, log_op
│   │   ├── embedding.py        # Embedding generation via sentence-transformers
│   │   └── logging.py          # Operation timing logging
│   ├── test_db.py              # Database layer unit/integration tests
│   └── test_e2e.py             # End-to-end tests
├── dashboard/                  # Next.js web UI frontend
│   ├── app/                    # Next.js App Router pages and layouts
│   │   ├── layout.tsx          # Root layout with AppProvider
│   │   ├── page.tsx            # Dashboard overview (stats, charts)
│   │   ├── globals.css         # Global styles
│   │   ├── search/             # Search results page
│   │   ├── entity/[id]/        # Entity detail page
│   │   ├── episode/[id]/       # Episode detail page
│   │   ├── episodes/           # Episodes list
│   │   ├── procedure/[id]/     # Procedure detail page
│   │   ├── procedures/         # Procedures list
│   │   ├── procedure/new/      # Create procedure page
│   │   ├── ingest/             # Ingest/import page
│   │   ├── graph/              # Graph visualization page
│   │   ├── maintenance/        # Maintenance/cleanup page
│   │   └── new/                # Create new entity page
│   ├── components/             # Reusable React components
│   ├── lib/                    # Utilities (GraphQL queries, formatting)
│   ├── package.json            # Next.js dependencies
│   ├── tsconfig.json           # TypeScript config
│   ├── tailwind.config.ts      # Tailwind CSS config
│   ├── postcss.config.js       # PostCSS config
│   └── next.config.js          # Next.js config
├── docs/                       # Documentation and diagrams
│   └── diagrams/               # Architecture diagrams
├── helm/                       # Kubernetes Helm charts
│   └── memcp/                  # Helm chart for memcp deployment
│       └── templates/          # K8s manifests
├── pyproject.toml              # Python package metadata and dependencies
├── uv.lock                     # UV lock file for dependency management
├── Dockerfile                  # Container image for memcp
├── docker-compose.yml          # Local development environment
├── supervisord.conf            # Process supervisor config (memcp + API)
├── test_memcp.py               # Unit test for module compilation
├── test_integration.py         # Integration tests with SurrealDB
├── test_graphql.py             # GraphQL resolver tests
├── README.md                   # Project overview and usage
├── ROADMAP.md                  # Feature roadmap
├── CLAUDE.md                   # Project-specific Claude instructions
└── webui-plan.md               # Web UI implementation plan

```

## Directory Purposes

**memcp/**
- Purpose: Main Python MCP server package
- Contains: Protocol implementation, database layer, sub-servers, utilities
- Key files: `server.py` (entry point), `db.py` (all queries), `servers/` (features)

**memcp/servers/**
- Purpose: Feature-organized sub-servers (mounted on main FastMCP server)
- Contains: 6 FastMCP sub-servers, each a complete tool domain
- Key files: `search.py`, `persist.py`, `episode.py`, `procedure.py`, `graph.py`, `maintenance.py`
- Pattern: Each file defines 1-3 tools with full docstrings

**memcp/api/**
- Purpose: Optional web interface (FastAPI + GraphQL)
- Contains: FastAPI app, GraphQL schema, resolver functions
- Key files: `main.py` (app setup), `schema.py` (type definitions)
- Usage: Enables dashboard UI, not required for MCP-only usage

**memcp/utils/**
- Purpose: Shared utilities used across sub-servers
- Contains: Embedding generation, contradiction detection, operation logging
- Key files: `embedding.py` (sentence-transformers integration), `logging.py` (metrics)

**dashboard/**
- Purpose: Next.js web UI for memory browsing and management
- Contains: React components, GraphQL queries, pages
- Key files: `app/page.tsx` (dashboard home), `lib/graphql.ts` (query definitions)
- Requires: Running API server at localhost:8000/graphql

**docs/**
- Purpose: Project documentation and architecture diagrams
- Contains: README, diagrams, technical reference
- Key files: `diagrams/` directory with architecture visuals

**helm/**
- Purpose: Kubernetes deployment manifests
- Contains: Helm chart with deployment, service, configmap templates
- Key files: `memcp/values.yaml` (deployment config)

## Key File Locations

**Entry Points:**
- `memcp/__init__.py`: Package entry point (imports server.main)
- `memcp/server.py`: MCP server startup (FastMCP app creation, sub-server mounting, lifespan)
- `memcp/api/main.py`: Web API startup (FastAPI + GraphQL router)
- `dashboard/app/page.tsx`: Web UI home page (dashboard overview)

**Configuration:**
- `pyproject.toml`: Python dependencies, pytest config, entry points
- `.env` (not in repo): Runtime config (SURREALDB_URL, credentials, etc.)
- `memcp/db.py` top 50 lines: Environment variable definitions
- `dashboard/lib/graphql.ts`: GraphQL endpoint URL configuration

**Core Logic:**
- `memcp/db.py`: ALL database queries (1300+ lines, query_* functions)
- `memcp/servers/*.py`: MCP tools and business logic
- `memcp/models.py`: Pydantic response type definitions
- `memcp/api/main.py`: GraphQL resolvers
- `memcp/utils/embedding.py`: Embedding generation with caching

**Testing:**
- `memcp/test_db.py`: Database query tests (unit + integration)
- `test_integration.py`: End-to-end MCP server tests
- `test_graphql.py`: GraphQL resolver tests
- `test_memcp.py`: Module compilation check

## Naming Conventions

**Python Files:**
- Sub-servers: `{feature}.py` (e.g., `search.py`, `persist.py`)
- Test files: `test_*.py` (e.g., `test_db.py`)
- Utilities: descriptive names (e.g., `embedding.py`, `logging.py`)

**Python Functions:**
- Query functions: `query_{operation}` (e.g., `query_hybrid_search()`, `query_upsert_entity()`)
- Tools (MCP): snake_case, verb-first (e.g., `search()`, `remember()`, `traverse()`)
- Internal utilities: snake_case (e.g., `embed()`, `validate_entity()`, `detect_context()`)
- MCP resource getters: `get_{resource}()` (e.g., `get_memory_stats()`)

**Python Classes:**
- Pydantic models: PascalCase, descriptive (e.g., `EntityResult`, `SearchResult`, `ReflectResult`)
- FastMCP instances: snake_case (e.g., `search_server = FastMCP("search")`)

**TypeScript/React:**
- Components: PascalCase (e.g., `StatCard`, `SearchResults`, `EntityDetail`)
- Pages: lowercase with [id] pattern (e.g., `page.tsx`, `[id]/page.tsx`)
- GraphQL queries: UPPERCASE (e.g., `QUERIES`, `GET_ENTITY_QUERY`)

**Database (SurrealDB):**
- Tables: lowercase, singular (e.g., `entity`, `episode`, `procedure`, `relates`, `extracted_from`)
- Fields: snake_case (e.g., `decay_weight`, `access_count`, `rel_type`)
- Indexes: descriptive lowercase (e.g., `entity_labels`, `episode_timestamp`)
- Analyzers: descriptive lowercase (e.g., `entity_analyzer`)

**Environment Variables:**
- SurrealDB config: `SURREALDB_*` (e.g., `SURREALDB_URL`, `SURREALDB_USER`)
- memcp config: `MEMCP_*` (e.g., `MEMCP_DEFAULT_CONTEXT`, `MEMCP_LOG_FILE`)

## Where to Add New Code

**New Search Feature:**
- Primary code: `memcp/servers/search.py` (add new tool function)
- Database layer: `memcp/db.py` (add query_* function)
- Model: Add to `memcp/models.py` if new return type needed
- Tests: `memcp/test_db.py` and `test_integration.py`

**New Memory Type (like Episode/Procedure):**
- Create: `memcp/servers/{type}.py` (new sub-server)
- Database: Add queries to `memcp/db.py` (tables defined in SCHEMA_SQL)
- Models: Add result types to `memcp/models.py`
- Register: Export from `memcp/servers/__init__.py` and mount in `memcp/server.py`
- API: Add GraphQL types to `memcp/api/schema.py` if needed

**New Utility Function:**
- Shared logic: `memcp/utils/{domain}.py` (create new file if needed)
- Helper: `memcp/utils/__init__.py` (add to __all__)
- Usage: Import in servers that need it

**Dashboard Page:**
- New page: `dashboard/app/{feature}/page.tsx` (use Next.js App Router)
- Components: Extract to `dashboard/components/` reusables
- GraphQL: Add query to `dashboard/lib/graphql.ts` QUERIES object
- Styling: Use Tailwind utility classes (configured in `tailwind.config.ts`)

**New Database Table:**
- Schema: `memcp/db.py` SCHEMA_SQL constant (add DEFINE TABLE ... block)
- Queries: `memcp/db.py` (add query_* functions for CRUD)
- Tests: `memcp/test_db.py` (add test_query_* tests)
- Lifespan: Update `app_lifespan()` if special initialization needed

## Special Directories

**memcp/servers/:**
- Purpose: Sub-server modules, each a complete feature domain
- Generated: No
- Committed: Yes (all source code)
- Structure: Each file is independent FastMCP("name") server

MemCP/api/:**
- Purpose: Optional web interface (not required for MCP-only)
- Generated: No (code is handwritten)
- Committed: Yes
- Notes: Only used if web UI needed; can be deployed separately

**dashboard/:**
- Purpose: Next.js frontend (independent deployment)
- Generated: node_modules/ (generated, not committed)
- Committed: Source code only
- Notes: Requires running API server; can run on different port/host

**.planning/codebase/:**
- Purpose: GSD codebase analysis documents
- Generated: Yes (generated by map-codebase command)
- Committed: No (git ignore)
- Notes: Used by GSD planning/execution commands

**helm/:**
- Purpose: Kubernetes deployment configuration
- Generated: No
- Committed: Yes
- Notes: Used for production deployments

---

*Structure analysis: 2026-02-01*
