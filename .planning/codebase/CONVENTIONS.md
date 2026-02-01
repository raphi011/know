# Coding Conventions

**Analysis Date:** 2026-02-01

## Naming Patterns

**Files:**
- `snake_case` for Python modules: `memcp/db.py`, `memcp/server.py`, `memcp/models.py`
- Test files use descriptive names: `test_integration.py`, `test_memcp.py`, `test_graphql.py`
- Sub-servers located in `memcp/servers/` with names matching their domain: `search.py`, `graph.py`, `persist.py`, `episode.py`, `procedure.py`, `maintenance.py`

**Functions:**
- `snake_case` throughout: `query_upsert_entity()`, `query_hybrid_search()`, `detect_context()`, `get_entity_types()`
- Database query functions prefixed with `query_`: `query_create_episode()`, `query_list_contexts()`, `query_apply_decay()`
- Tool functions use descriptive names: `search()`, `remember()`, `traverse()`, `reflect()`, `check_contradictions_tool()`
- Private/internal functions prefixed with underscore: `_get_embedder()`, `_get_nli_model()`, `_get_git_origin_name()`

**Variables:**
- `snake_case` for local variables: `entity_results`, `query_embedding`, `effective_context`
- Constants in `UPPER_SNAKE_CASE`: `SURREALDB_URL`, `QUERY_TIMEOUT`, `ENTITY_TYPES`, `ALLOW_CUSTOM_TYPES`, `TEST_DATABASE`
- Environment variables prefixed: `MEMCP_*`, `SURREALDB_*`
- Type alias names descriptive: `DBConnection`, `QueryResult`

**Types/Classes:**
- `PascalCase` for Pydantic models: `EntityResult`, `SearchResult`, `EpisodeResult`, `ContextStats`, `ProcedureResult`
- Descriptive model names match their purpose: `ReflectResult`, `RememberResult`, `MemoryStats`, `SimilarPair`

## Code Style

**Formatting:**
- 4-space indentation (Python standard)
- Line length: no explicit limit enforced, but functions are kept reasonable
- No black/prettier config in repo; uses Python defaults

**Linting:**
- No explicit linter config found (ruff/pylint not configured)
- Code follows PEP 8 conventions
- Type hints used throughout: `def detect_context(explicit_context: str | None = None) -> str | None:`
- Union syntax uses `|` operator (Python 3.10+): `str | None` instead of `Optional[str]`

## Import Organization

**Order:**
1. Standard library: `import asyncio`, `import os`, `from datetime import datetime`
2. Third-party: `from fastmcp import Context, FastMCP`, `from surrealdb import AsyncSurreal`, `from pydantic import BaseModel`
3. Local/relative: `from memcp.db import query_hybrid_search`, `from memcp.models import EntityResult`

**Path Aliases:**
- No path aliases configured (all imports use absolute paths from `memcp` root)
- Imports follow module hierarchy: `from memcp.servers.search import search`

**Example from `memcp/servers/search.py`:**
```python
import time
from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import EntityResult, SearchResult, ContextStats, ContextListResult, EntityTypeInfo, EntityTypeListResult
from memcp.utils import embed, log_op, extract_record_id
from memcp.db import (
    detect_context, get_db,
    query_hybrid_search, query_update_access, query_get_entity, query_list_labels,
    query_list_contexts, query_get_context_stats,
    query_list_types, query_entities_by_type,
    get_entity_types, ALLOW_CUSTOM_TYPES
)
```

## Error Handling

**Patterns:**
- Use `fastmcp.exceptions.ToolError` for user-facing errors: `raise ToolError("Query cannot be empty")`
- Validation checks at tool entry points: query length, limit bounds, type validation
- Null/None handling with `or` pattern: `embedding = embed(content) or [0.0] * 384`
- Database errors allow stack traces to propagate (caught at server level)
- Optional fields use `option<type>` in SurrealDB schema with `None` defaults

**Example from `memcp/db.py`:**
```python
def validate_entity_type(entity_type: str) -> str:
    """Validate and normalize entity type."""
    normalized = entity_type.lower().strip()

    if normalized in ENTITY_TYPES:
        return normalized

    if ALLOW_CUSTOM_TYPES:
        return normalized

    valid_types = ", ".join(sorted(ENTITY_TYPES.keys()))
    raise ToolError(f"Invalid entity type '{entity_type}'. Valid types: {valid_types}")
```

## Logging

**Framework:** Built-in Python `logging` module

**Patterns:**
- Module-level logger: `logger = logging.getLogger("memcp.db")`
- Log operations with timing: `log_op('search', start, query=query[:30], limit=limit, results=len(results))`
- Info-level logging for operations: `await ctx.info(f"Searching for: {query[:50]}...")`
- Error logging via context: `await ctx.error(msg)`
- File logging to `/tmp/memcp.log` with level controlled by `MEMCP_LOG_LEVEL` env var

**Operation logging format from `memcp/utils/logging.py`:**
```python
def log_op(operation: str, start_time: float, **details) -> None:
    """Log operation with timing and details."""
    duration_ms = (time.time() - start_time) * 1000
    detail_str = ' '.join(f'{k}={v}' for k, v in details.items())
    logger.info(f"[{operation}] {duration_ms:.1f}ms {detail_str}")
```

## Comments

**When to Comment:**
- Docstrings on all public functions with parameter descriptions and return types
- Inline comments for non-obvious logic or workarounds
- Comments in SurrealDB schema sections with divider lines: `# ==========================================================================`
- Comments explaining TODO items and known issues: `# TODO: Use set<string> when Python SDK supports CBOR tag 56`
- Comments explaining query result format variations (important due to SurrealDB SDK nuances)

**JSDoc/Docstring:**
- Google-style docstrings for functions:
```python
def detect_context(explicit_context: str | None = None) -> str | None:
    """Detect project context from explicit value, env, git origin, or cwd.

    Priority:
    1. Explicit context parameter (if provided)
    2. MEMCP_DEFAULT_CONTEXT env var (if set)
    3. Git remote origin name (if MEMCP_CONTEXT_FROM_CWD=true and in a git repo)
    4. Current working directory basename (if MEMCP_CONTEXT_FROM_CWD=true)
    5. None (no context filtering)
    """
```

- Tool docstrings include usage guidance:
```python
@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    context: str | None = None,
    ctx: Context = None
) -> SearchResult:
    """Search your persistent memory for previously stored knowledge. Use when the user asks 'do you remember...', 'what do you know about...', 'recall...', or needs context from past conversations. Combines semantic similarity and keyword matching.

    Args:
        query: The search query
        labels: Optional list of labels to filter by
        limit: Max results (1-100)
        context: Optional project namespace to filter by...
    """
```

## Function Design

**Size:**
- Functions kept to single responsibility (20-100 lines typical)
- Query functions handle parameter validation + database call + result parsing
- Tools handle input validation, context detection, operation, and result serialization
- Async throughout for database operations

**Parameters:**
- Named parameters for clarity: `query_upsert_entity(db, entity_id=..., entity_type=..., labels=...)`
- Optional parameters use `None` as default: `context: str | None = None`
- MCP Context parameter always named `ctx` and comes last
- Database connection passed as first parameter: `async def query_get_entity(db: DBConnection, entity_id: str)`

**Return Values:**
- Query functions return `QueryResult` (list of dicts from SurrealDB)
- Tools return Pydantic model instances: `SearchResult`, `EntityResult`, `ReflectResult`
- Methods returning nothing have no return statement (implicit None)
- Async functions always declared with `async def`

## Module Design

**Exports:**
- Functions exported explicitly (no `__all__` but implicit through imports)
- Classes exported: all Pydantic models from `memcp/models.py`
- Constants exported: `ENTITY_TYPES`, `SCHEMA_SQL`, configuration variables

**Barrel Files:**
- `memcp/__init__.py` exports main entry point: `def main():`
- `memcp/servers/__init__.py` imports sub-servers: `from . import search_server, graph_server, persist_server`
- Utilities in `memcp/utils/` with clear separation: `embedding.py`, `logging.py`

**Utilities Pattern:**
- `memcp/utils/__init__.py` imports and re-exports: `from .embedding import embed`
- Each utility module focused: embedding handles ML models, logging handles operation tracing
- Lazy loading for expensive resources (embedding model loaded on first use)

## Type Hints

**Coverage:**
- Type hints on all function signatures
- Generic types used: `list[str]`, `dict[str, int]`, `list[EntityResult]`
- Optional types explicit: `str | None` throughout
- No `Any` types except in specific locations (result parsing from SurrealDB)
- Pydantic models use field types: `labels: list[str] = Field(default_factory=list)`

## Async Patterns

**Conventions:**
- All database operations are async: `async def query_*`
- All tools are async: `async def search(...)`
- Context operations may be async: `await ctx.info(msg)`, `await ctx.error(msg)`
- Fixtures use `@pytest_asyncio.fixture` and `async def`
- Tests use `@pytest.mark.asyncio` decorator

---

*Convention analysis: 2026-02-01*
