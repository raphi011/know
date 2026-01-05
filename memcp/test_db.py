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

import pytest
from surrealdb import AsyncSurreal

from memcp.db import SCHEMA_SQL


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


@pytest.fixture(scope="session")
async def db_session(test_db_config):
    """Session-scoped database connection."""
    db = AsyncSurreal(test_db_config['url'])

    try:
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

        yield db

    finally:
        # Cleanup: drop test database
        try:
            await db.query(f"REMOVE DATABASE {test_db_config['database']}")
            await db.close()
        except Exception:
            pass


@pytest.fixture
async def clean_db(db_session):
    """Function-scoped fixture that cleans database before each test."""
    # Clean all entities before test
    await db_session.query("DELETE entity")
    yield db_session
    # Clean after test
    await db_session.query("DELETE entity")


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
