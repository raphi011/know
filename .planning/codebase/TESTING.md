# Testing Patterns

**Analysis Date:** 2026-02-01

## Test Framework

**Runner:**
- pytest (v9.0.2+)
- pytest-asyncio (v1.3.0+) for async test support
- pytest-timeout (v2.4.0+) for test timeouts

**Config:** `pyproject.toml` in `[tool.pytest.ini_options]`
```toml
asyncio_mode = "auto"
asyncio_default_fixture_loop_scope = "function"
timeout = 30
timeout_method = "thread"
markers = [
    "integration: marks tests as integration tests (require SurrealDB)",
    "embedding: marks tests that require embedding models (slow to load)",
]
```

**Assertion Library:** pytest assertions (built-in, no custom library)

**Run Commands:**
```bash
# Unit test (fast, ensures module compiles)
uv run pytest test_memcp.py -v

# Integration tests (requires SurrealDB running)
uv run pytest test_integration.py -v

# Skip integration tests when no database
uv run pytest test_integration.py -v -m "not integration"

# Run with markers
uv run pytest -m "embedding" -v  # Only tests marked @pytest.mark.embedding
uv run pytest -m "integration" -v  # Only integration tests

# With coverage (if coverage plugin installed)
uv run pytest --cov=memcp test_integration.py -v
```

## Test File Organization

**Location:**
- Test files in repo root next to source: `test_memcp.py`, `test_integration.py`, `test_graphql.py`, `test_e2e.py`
- Database-specific tests in `memcp/test_db.py` (legacy, co-located)
- E2E tests in `memcp/test_e2e.py` (co-located)

**Naming:**
- Test files: `test_*.py` (pytest discovery)
- Test classes: `Test*` (e.g., `TestEntityQueries`, `TestPersistTools`)
- Test functions: `test_*` (e.g., `test_upsert_entity_with_context`)

**Structure:**
```
test_integration.py
├── Test fixtures (pytest_asyncio fixtures)
├── TestContextDetection (test class)
│   ├── test_detect_context_explicit()
│   └── test_detect_context_none()
├── TestEntityQueries (test class with @pytest.mark.embedding)
│   ├── test_upsert_entity_with_context()
│   ├── test_upsert_entity_with_importance()
│   └── ...
├── TestPersistTools (test class with @pytest.mark.embedding)
└── Fixture definitions (mock_ctx, cleanup_test_data, db_connection)
```

## Test Structure

**Suite Organization:**
Tests organized into test classes grouped by feature area. Each class tests a related set of functionality. Section headers with comment dividers organize tests by layer (Entity tests, Episode tests, etc.).

**Patterns:**

1. **Async fixtures with proper setup/teardown:**
```python
@pytest_asyncio.fixture
async def db_connection():
    """Set up database connection for tests."""
    from surrealdb import AsyncSurreal
    from memcp.db import (
        SURREALDB_URL, SURREALDB_NAMESPACE, SURREALDB_DATABASE,
        SURREALDB_USER, SURREALDB_PASS, SCHEMA_SQL
    )

    db = AsyncSurreal(SURREALDB_URL)
    try:
        await db.connect()
        await db.signin({"username": SURREALDB_USER, "password": SURREALDB_PASS})
        await db.use(SURREALDB_NAMESPACE, SURREALDB_DATABASE)
        await db.query(SCHEMA_SQL)
        yield db
    except Exception as e:
        pytest.skip(f"SurrealDB not available: {e}")
    finally:
        pass  # Don't await db.close() - it hangs due to SDK race condition
```

2. **Auto-used fixtures for cleanup:**
```python
@pytest_asyncio.fixture(autouse=True)
async def cleanup_test_data(db_connection):
    """Clean up test data before and after each test."""
    # Clean up before test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
    yield
    # Clean up after test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
```

3. **Mock context factory:**
```python
@pytest_asyncio.fixture
async def mock_ctx(db_connection):
    """Create a mock context for query functions."""
    from dataclasses import dataclass
    from typing import Any

    @dataclass
    class MockRequestContext:
        lifespan_context: Any

    @dataclass
    class MockAppContext:
        db: Any
        initialized: bool = True

    class MockContext:
        def __init__(self, db):
            self.request_context = MockRequestContext(
                lifespan_context=MockAppContext(db=db)
            )

        async def info(self, msg: str):
            pass

        async def error(self, msg: str):
            pass

    return MockContext(db_connection)
```

4. **Unit tests without database:**
```python
class TestModels:
    def test_entity_result_with_new_fields(self):
        """Test EntityResult includes context and importance."""
        from memcp.models import EntityResult

        entity = EntityResult(
            id="test:123",
            content="Test content",
            context="my-project",
            importance=0.75
        )

        assert entity.context == "my-project"
        assert entity.importance == 0.75
```

## Mocking

**Framework:** unittest.mock (standard library)

**Patterns:**

1. **Mock context for tools:**
```python
from unittest.mock import AsyncMock, MagicMock

ctx = MagicMock()
ctx.request_context.lifespan_context.db = clean_db
ctx.info = AsyncMock()
ctx.error = AsyncMock()
```

2. **Mocking in MCP client tests:**
```python
from fastmcp import Client

@pytest_asyncio.fixture
async def mcp_client():
    """Create MCP client connected to server."""
    from memcp.server import mcp

    async with Client(mcp) as client:
        yield client
```

**What to Mock:**
- MCP Context objects (database connection, logging methods)
- Resource reading for tools that call `ctx.read_resource()`
- Expensive operations that aren't being tested (embeddings, NLI models)

**What NOT to Mock:**
- Database operations in integration tests (use real SurrealDB)
- Embedding functions in tests that require semantic search
- Query functions (test them against real database)

## Fixtures and Factories

**Test Data:**

Embeddings created consistently as fixed-size vectors (384 dimensions required by schema):
```python
test_embedding = [0.1] * 384
similar_embedding = [0.5] * 384
different_embedding = [0.1] * 384
```

Entity creation pattern:
```python
from memcp.utils import embed

embedding = embed("test content for context")
await query_upsert_entity(
    db_connection,
    entity_id="test_context_entity",
    entity_type="test",
    labels=["test"],
    content="test content for context",
    embedding=embedding,
    confidence=1.0,
    source="test",
    context="test-project"
)
```

**Location:**
- Fixtures defined at top of test modules (after imports, before test classes)
- Database fixtures in `test_integration.py` and `memcp/test_db.py`
- Mock context fixtures in both integration and E2E tests
- Session and function-scoped fixtures: `@pytest.fixture(scope="session")` vs `@pytest_asyncio.fixture`

## Coverage

**Requirements:** No explicit coverage requirement enforced

**View Coverage:**
```bash
# Install coverage plugin if needed
uv add --dev pytest-cov

# Generate coverage report
uv run pytest test_integration.py --cov=memcp --cov-report=html
```

## Test Types

**Unit Tests:** `test_memcp.py`
- Scope: Module compilation and imports
- No database required
- Fast (<1s)
- Example:
```python
def test_module_compiles():
    """Verify memcp module and submodules compile and can be imported."""
    import memcp
    import memcp.server
    import memcp.db
    import memcp.models

    assert memcp.main is not None
    assert memcp.server.mcp is not None
    assert memcp.db.run_query is not None
    assert memcp.models.EntityResult is not None
```

**Integration Tests:** `test_integration.py`
- Scope: Database queries, tools with real SurrealDB
- Requires: Running SurrealDB instance (`docker run -p 8000:8000 surrealdb/surrealdb:latest ...`)
- Marked: `@pytest.mark.integration`, `@pytest.mark.embedding` (slow, loads ML models)
- Timeout: 30 seconds per test
- Setup: Schema initialized, test data cleaned before/after each test
- Example:
```python
@pytest.mark.embedding
class TestEntityQueries:
    async def test_upsert_entity_with_context(self, db_connection):
        """Test entity creation with context."""
        from memcp.db import query_upsert_entity, query_get_entity
        from memcp.utils import embed

        embedding = embed("test content for context")
        await query_upsert_entity(
            db_connection,
            entity_id="test_context_entity",
            entity_type="test",
            labels=["test"],
            content="test content for context",
            embedding=embedding,
            confidence=1.0,
            source="test",
            context="test-project"
        )

        result = await query_get_entity(db_connection, "test_context_entity")
        assert len(result) == 1
        assert result[0].get('context') == "test-project"
```

**E2E Tests:** `memcp/test_e2e.py`
- Scope: MCP protocol integration, tool serialization, full workflow
- Uses: MCP Client against running server
- Marked: `@pytest.mark.asyncio`
- Example:
```python
@pytest.mark.asyncio
async def test_remember_and_get_entity(mcp_client: Client):
    """Test storing and retrieving an entity via MCP."""
    # Remember
    result = await mcp_client.call_tool("remember", {
        "id": "e2e-test-entity",
        "content": "E2E test content for MCP integration",
        "type": "test",
        "labels": ["e2e", "integration"],
        "weight": 1.0,
        "source": "test_e2e.py"
    })

    assert result.is_error is False

    # Get entity
    result = await mcp_client.call_tool("get_entity", {
        "entity_id": "e2e-test-entity"
    })

    assert result.is_error is False
```

## Common Patterns

**Async Testing:**
```python
@pytest.mark.asyncio
async def test_query_something(mock_ctx):
    """Test an async query function."""
    result = await query_function(mock_ctx, param="value")
    assert result is not None
    assert len(result) > 0
```

**Error Testing:**
```python
def test_validate_entity_type_invalid():
    """Test validation rejects invalid types."""
    from memcp.db import validate_entity_type
    from fastmcp.exceptions import ToolError

    with pytest.raises(ToolError):
        validate_entity_type("invalid_type")
```

**Null/Optional Field Testing:**
```python
async def test_entity_with_optional_fields(self, db_connection):
    """Test entity creation with optional fields as None."""
    from memcp.db import query_upsert_entity, query_get_entity
    from memcp.utils import embed

    embedding = embed("test")
    await query_upsert_entity(
        db_connection,
        entity_id="test_optional",
        entity_type="test",
        labels=[],
        content="test",
        embedding=embedding,
        confidence=1.0,
        source=None,  # Optional field
        context=None,  # Optional field
        user_importance=None  # Optional field
    )

    result = await query_get_entity(db_connection, "test_optional")
    assert result[0].get('source') is None
    assert result[0].get('context') is None
```

**Testing Result Format Variations:**
SurrealDB SDK returns results in different formats. Tests handle both:
```python
# Handle both result formats
if isinstance(result[0], list):
    created = result[0][0]
else:
    created = result[0]

assert isinstance(created, dict), f"Expected dict, got {type(created)}"
```

**Testing Tool Parameters:**
```python
async def test_search_tool_with_context(self, mock_ctx, db_connection):
    """Test search tool filters by context."""
    from memcp.servers.search import search
    from memcp.db import query_upsert_entity
    from memcp.utils import embed

    await query_upsert_entity(
        db_connection, "test_ctx_a", "concept", [],
        "Entity in project A", embed("Entity in project A"), 1.0, "test",
        context="project-a"
    )

    result = await search.fn("project", context="project-a", ctx=mock_ctx)

    assert all(e.context == "project-a" for e in result.entities if e.context)
```

## Test Organization Strategy

**By Layer:**
1. Model tests (unit): `TestModels`, `TestProcedureModels` - test Pydantic validation
2. Query function tests (integration): `TestEntityQueries`, `TestEpisodeQueries` - test database operations
3. Tool tests (integration): `TestSearchTools`, `TestPersistTools` - test MCP tool contract
4. E2E tests: Full workflow through MCP protocol

**By Feature:**
- Context detection tests
- Entity CRUD tests (upsert, get, delete)
- Search tests (hybrid, by type, by label)
- Episode tests (create, search, link)
- Procedure tests (CRUD, search, list)
- Maintenance tests (reflect, decay, importance)
- Contradiction detection tests

**Marker Usage:**
- `@pytest.mark.integration` - requires running database
- `@pytest.mark.embedding` - loads expensive ML models (combine with integration)
- `@pytest.mark.asyncio` - async function
- `@pytest.mark.skip(reason="...")` - temporarily skip test

---

*Testing analysis: 2026-02-01*
