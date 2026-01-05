"""Integration tests for memcp database operations.

These tests run against a real SurrealDB instance.

Setup:
    docker run -p 8000:8000 surrealdb/surrealdb:latest \
      start --user root --pass root memory

Environment:
    export SURREALDB_TEST_URL="ws://localhost:8000/rpc"
    export SURREALDB_TEST_USER="root"
    export SURREALDB_TEST_PASS="root"
"""

import asyncio
import os
import time
from typing import AsyncIterator
from unittest.mock import AsyncMock, MagicMock

import pytest
import pytest_asyncio
from surrealdb import AsyncSurreal

from memcp.db import (
    SCHEMA_SQL,
    query_upsert_entity,
    query_get_entity,
    query_delete_entity,
)


# Test configuration
TEST_URL = os.getenv("SURREALDB_TEST_URL", "ws://localhost:8000/rpc")
TEST_USER = os.getenv("SURREALDB_TEST_USER", "root")
TEST_PASS = os.getenv("SURREALDB_TEST_PASS", "root")
TEST_NAMESPACE = "knowledge_test"
TEST_DATABASE = f"graph_test_{int(time.time())}"


@pytest.fixture(scope="session")
def test_db_config():
    """Test database configuration."""
    return {
        'url': TEST_URL,
        'namespace': TEST_NAMESPACE,
        'database': TEST_DATABASE,
        'user': TEST_USER,
        'pass': TEST_PASS,
    }


@pytest_asyncio.fixture
async def clean_db(test_db_config):
    """Function-scoped database connection that cleans between tests."""
    db = AsyncSurreal(test_db_config['url'])

    # Connect
    await asyncio.wait_for(db.connect(), timeout=10)

    # Authenticate
    await db.signin({
        "username": test_db_config['user'],
        "password": test_db_config['pass']
    })

    # Use test namespace/database
    await db.use(test_db_config['namespace'], test_db_config['database'])

    # Initialize schema
    await db.query(SCHEMA_SQL)

    # Clean before test
    await db.query("DELETE entity")

    yield db

    # Clean after test
    await db.query("DELETE entity")
    await db.close()


# =============================================================================
# Connection Tests
# =============================================================================

@pytest.mark.asyncio
async def test_connection(test_db_config):
    """Test basic database connection."""
    db = AsyncSurreal(test_db_config['url'])

    await db.connect()
    await db.signin({
        "username": test_db_config['user'],
        "password": test_db_config['pass']
    })
    await db.use(test_db_config['namespace'], test_db_config['database'])

    # Verify connection works - use info command
    result = await db.query("INFO FOR DB")
    assert result is not None

    await db.close()


# =============================================================================
# Query Function Tests
# =============================================================================

@pytest_asyncio.fixture
async def mock_ctx(clean_db):
    """Create mock Context for query functions."""
    ctx = MagicMock()
    # Mock the context to return our test DB connection
    ctx.request_context.lifespan_context.db = clean_db
    # Mock logging methods
    ctx.info = AsyncMock()
    ctx.error = AsyncMock()
    return ctx


@pytest.mark.asyncio
async def test_query_upsert_and_get_entity(mock_ctx):
    """Test query_upsert_entity and query_get_entity functions."""
    # Create entity using our query function
    # Create 384-dimensional embedding (required by schema)
    test_embedding = [0.1] * 384

    result = await query_upsert_entity(
        mock_ctx,
        entity_id="test_user",
        entity_type="person",
        labels=["test", "user"],
        content="Test user entity",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Result format from SurrealDB: list[list[dict]] or list[dict]
    assert result is not None
    assert len(result) > 0

    # Handle both result formats
    if isinstance(result[0], list):
        created = result[0][0]
    else:
        created = result[0]

    assert isinstance(created, dict), f"Expected dict, got {type(created)}"
    assert created['type'] == 'person'
    assert created['content'] == 'Test user entity'

    # Read entity using our query function
    result = await query_get_entity(mock_ctx, "test_user")

    assert result is not None
    assert len(result) > 0
    # Result[0] is the entity dict
    entity = result[0]
    assert str(entity['id']) == 'entity:test_user'
    assert entity['type'] == 'person'
    assert entity['content'] == 'Test user entity'
    assert entity['labels'] == ['test', 'user']


@pytest.mark.asyncio
async def test_query_delete_entity(mock_ctx):
    """Test query_delete_entity function."""
    # Create entity first
    test_embedding = [0.1] * 384
    await query_upsert_entity(
        mock_ctx,
        entity_id="to_delete",
        entity_type="temp",
        labels=["test"],
        content="Will be deleted",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Verify it exists
    result = await query_get_entity(mock_ctx, "to_delete")
    assert result is not None
    assert len(result) > 0

    # Delete using our query function
    result = await query_delete_entity(mock_ctx, "to_delete")

    # Verify deletion
    result = await query_get_entity(mock_ctx, "to_delete")
    assert len(result) == 0 or (len(result) > 0 and len(result[0]) == 0)
