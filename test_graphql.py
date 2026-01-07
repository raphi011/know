"""Integration tests for GraphQL API.

These tests verify the GraphQL resolvers return correct data from SurrealDB.
Run with: uv run pytest test_graphql.py -v

Requires: Running SurrealDB instance
"""

import asyncio
import os
import pytest
import pytest_asyncio
from datetime import datetime, timedelta

import strawberry
from strawberry.test import Response

pytestmark = pytest.mark.integration

# Check if SurrealDB is available
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")


# =============================================================================
# Test Fixtures
# =============================================================================


@pytest_asyncio.fixture
async def db():
    """Create isolated test database connection."""
    from surrealdb import AsyncSurreal
    from memcp.db import (
        SURREALDB_URL,
        SURREALDB_USER,
        SURREALDB_PASS,
        SCHEMA_SQL,
    )

    db = AsyncSurreal(SURREALDB_URL)
    try:
        await db.connect()
        await db.signin({"username": SURREALDB_USER, "password": SURREALDB_PASS})
        # Use separate test database
        await db.use("test", "graphql_test")
        # Initialize schema
        await db.query(SCHEMA_SQL)
        yield db
    except Exception as e:
        pytest.skip(f"SurrealDB not available: {e}")
    finally:
        # Clean up test database
        try:
            await db.query("REMOVE DATABASE IF EXISTS graphql_test")
        except Exception:
            pass


@pytest.fixture
def schema():
    """Create Strawberry schema from Query and Mutation."""
    from memcp.api.main import Query, Mutation

    return strawberry.Schema(query=Query, mutation=Mutation)


@pytest_asyncio.fixture
async def setup_db_connection(db):
    """Inject db connection into the web module's global _db."""
    import memcp.api.main as web_module

    # Store original
    original_db = web_module._db

    # Inject test db
    web_module._db = db

    yield db

    # Restore original
    web_module._db = original_db


@pytest_asyncio.fixture(autouse=True)
async def cleanup(db):
    """Clean up test data before and after each test."""
    # Clean before
    await db.query("DELETE entity")
    await db.query("DELETE episode")
    await db.query("DELETE procedure")
    await db.query("DELETE relates")
    yield
    # Clean after
    await db.query("DELETE entity")
    await db.query("DELETE episode")
    await db.query("DELETE procedure")
    await db.query("DELETE relates")


# =============================================================================
# Helper Functions
# =============================================================================


async def seed_entity(db, entity_id: str, entity_type: str = "concept", content: str = "Test content", context: str = None, labels: list[str] = None):
    """Seed a test entity."""
    from memcp.utils import embed

    embedding = embed(content)
    labels = labels or []

    await db.query(
        """
        CREATE type::record("entity", $id) SET
            type = $type,
            content = $content,
            embedding = $embedding,
            labels = $labels,
            context = $context,
            confidence = 1.0,
            importance = 0.5,
            access_count = 0,
            created = time::now(),
            accessed = time::now()
        """,
        {
            "id": entity_id,
            "type": entity_type,
            "content": content,
            "embedding": embedding,
            "labels": labels,
            "context": context,
        },
    )


async def seed_episode(db, episode_id: str, content: str = "Test episode", context: str = None, summary: str = None):
    """Seed a test episode."""
    from memcp.utils import embed

    embedding = embed(content)

    await db.query(
        """
        CREATE type::record("episode", $id) SET
            content = $content,
            embedding = $embedding,
            summary = $summary,
            context = $context,
            timestamp = time::now(),
            created = time::now(),
            accessed = time::now(),
            access_count = 0,
            metadata = {}
        """,
        {
            "id": episode_id,
            "content": content,
            "embedding": embedding,
            "summary": summary,
            "context": context,
        },
    )


async def seed_procedure(db, proc_id: str, name: str, description: str, steps: list[dict], context: str = None, labels: list[str] = None):
    """Seed a test procedure."""
    from memcp.utils import embed

    labels = labels or []
    embedding = embed(f"{name} {description}")

    await db.query(
        """
        CREATE type::record("procedure", $id) SET
            name = $name,
            description = $description,
            steps = $steps,
            embedding = $embedding,
            labels = $labels,
            context = $context,
            created = time::now(),
            accessed = time::now(),
            access_count = 0
        """,
        {
            "id": proc_id,
            "name": name,
            "description": description,
            "steps": steps,
            "embedding": embedding,
            "labels": labels,
            "context": context,
        },
    )


async def seed_relation(db, from_id: str, to_id: str, rel_type: str = "relates_to", weight: float = 1.0):
    """Seed a test relation."""
    await db.query(
        """
        LET $from_rec = type::record("entity", $from);
        LET $to_rec = type::record("entity", $to);
        RELATE $from_rec->relates->$to_rec SET
            rel_type = $rel_type,
            weight = $weight,
            created = time::now()
        """,
        {"from": from_id, "to": to_id, "rel_type": rel_type, "weight": weight},
    )


async def execute_query(schema, query: str, variables: dict = None):
    """Execute a GraphQL query and return data."""
    result = await schema.execute(query, variable_values=variables)
    if result.errors:
        raise Exception(f"GraphQL errors: {result.errors}")
    return result.data


# =============================================================================
# Query Tests
# =============================================================================


@pytest.mark.embedding
class TestContextsQuery:
    async def test_contexts_returns_unique_contexts(self, schema, setup_db_connection, db):
        """Test contexts query returns unique context names."""
        # Seed entities in different contexts
        await seed_entity(db, "e1", context="project-a")
        await seed_entity(db, "e2", context="project-b")
        await seed_entity(db, "e3", context="project-a")  # duplicate

        data = await execute_query(schema, "{ contexts }")

        contexts = data["contexts"]
        assert "project-a" in contexts
        assert "project-b" in contexts
        assert len(contexts) == 2  # Should be unique


@pytest.mark.embedding
class TestOverviewQuery:
    async def test_overview_returns_stats(self, schema, setup_db_connection, db):
        """Test overview returns entity/relation/episode counts."""
        # Seed data
        await seed_entity(db, "e1")
        await seed_entity(db, "e2")
        await seed_relation(db, "e1", "e2")
        await seed_episode(db, "ep1")

        data = await execute_query(schema, """
            {
                overview {
                    stats { title value }
                }
            }
        """)

        stats = data["overview"]["stats"]
        assert len(stats) >= 3

        # Find entity count stat (title is "Total Entities")
        entity_stat = next((s for s in stats if "Entities" in s["title"]), None)
        assert entity_stat is not None
        assert entity_stat["value"] == "2"

    async def test_overview_returns_type_distribution(self, schema, setup_db_connection, db):
        """Test overview returns entity type breakdown."""
        await seed_entity(db, "e1", entity_type="preference")
        await seed_entity(db, "e2", entity_type="preference")
        await seed_entity(db, "e3", entity_type="fact")

        data = await execute_query(schema, """
            {
                overview {
                    distribution { label val }
                }
            }
        """)

        distribution = data["overview"]["distribution"]
        assert len(distribution) >= 2

        # Check preference count
        pref_dist = next((d for d in distribution if d["label"].lower() == "preference"), None)
        assert pref_dist is not None
        assert pref_dist["val"] == 2


@pytest.mark.embedding
class TestRecentMemoriesQuery:
    async def test_recent_memories_ordered_by_accessed(self, schema, setup_db_connection, db):
        """Test recent memories are ordered by accessed time."""
        # Seed entities with different access times
        await seed_entity(db, "e1", content="First entity")
        await seed_entity(db, "e2", content="Second entity")

        # Update access times to ensure ordering
        await db.query("""
            UPDATE entity:e1 SET accessed = <datetime>"2024-01-01T00:00:00Z";
            UPDATE entity:e2 SET accessed = <datetime>"2024-01-02T00:00:00Z";
        """)

        data = await execute_query(schema, """
            {
                recentMemories(limit: 2) {
                    id
                    content
                }
            }
        """)

        memories = data["recentMemories"]
        assert len(memories) == 2
        # Most recent (e2) should be first
        assert "Second" in memories[0]["content"]
        assert "First" in memories[1]["content"]


@pytest.mark.embedding
class TestEpisodesQuery:
    async def test_episodes_filtered_by_context(self, schema, setup_db_connection, db):
        """Test episodes query filters by context."""
        await seed_episode(db, "ep1", content="Episode A", context="proj-a")
        await seed_episode(db, "ep2", content="Episode B", context="proj-b")

        data = await execute_query(
            schema,
            '{ episodes(context: "proj-a") { id content } }',
        )

        episodes = data["episodes"]
        assert len(episodes) == 1
        assert "Episode A" in episodes[0]["content"]

    async def test_episode_returns_metadata(self, schema, setup_db_connection, db):
        """Test single episode query returns metadata."""
        await seed_episode(db, "ep1", content="Test content", summary="Test summary", context="test")

        data = await execute_query(
            schema,
            '{ episode(id: "ep1") { id content summary context accessCount } }',
        )

        episode = data["episode"]
        assert episode is not None
        assert episode["content"] == "Test content"
        assert episode["summary"] == "Test summary"
        assert episode["context"] == "test"


@pytest.mark.embedding
class TestProceduresQuery:
    async def test_procedures_filtered_by_context(self, schema, setup_db_connection, db):
        """Test procedures query filters by context."""
        await seed_procedure(db, "p1", "Proc A", "Description A", [{"content": "Step 1"}], context="proj-a")
        await seed_procedure(db, "p2", "Proc B", "Description B", [{"content": "Step 1"}], context="proj-b")

        data = await execute_query(
            schema,
            '{ procedures(context: "proj-a") { id name } }',
        )

        procedures = data["procedures"]
        assert len(procedures) == 1
        assert procedures[0]["name"] == "Proc A"

    async def test_procedure_returns_steps(self, schema, setup_db_connection, db):
        """Test single procedure returns steps array."""
        steps = [
            {"content": "Step 1", "optional": False},
            {"content": "Step 2", "optional": True},
        ]
        await seed_procedure(db, "p1", "Deploy", "Deploy app", steps)

        data = await execute_query(
            schema,
            '{ procedure(id: "p1") { id name steps { content optional } } }',
        )

        proc = data["procedure"]
        assert proc is not None
        assert proc["name"] == "Deploy"
        assert len(proc["steps"]) == 2
        assert proc["steps"][0]["content"] == "Step 1"
        assert proc["steps"][1]["optional"] is True


@pytest.mark.embedding
class TestSearchMemoriesQuery:
    async def test_search_memories_returns_results(self, schema, setup_db_connection, db):
        """Test search returns matching entities."""
        await seed_entity(db, "e1", content="Python programming language")
        await seed_entity(db, "e2", content="JavaScript runtime")

        data = await execute_query(
            schema,
            '{ searchMemories(query: "Python") { id content score } }',
        )

        results = data["searchMemories"]
        assert len(results) >= 1
        # Python result should be found
        assert any("Python" in r["content"] for r in results)

    async def test_search_memories_filters_by_type(self, schema, setup_db_connection, db):
        """Test search filters by entity type."""
        await seed_entity(db, "e1", entity_type="preference", content="I prefer dark mode")
        await seed_entity(db, "e2", entity_type="fact", content="Dark mode reduces eye strain")

        data = await execute_query(
            schema,
            '{ searchMemories(query: "dark mode", type: "preference") { id type content } }',
        )

        results = data["searchMemories"]
        # Should only return preference type
        assert all(r["type"] == "preference" for r in results)


@pytest.mark.embedding
class TestEntityQuery:
    async def test_entity_returns_neighbors(self, schema, setup_db_connection, db):
        """Test entity query returns related neighbors."""
        await seed_entity(db, "e1", content="Main entity")
        await seed_entity(db, "e2", content="Related entity")
        await seed_relation(db, "e1", "e2")

        data = await execute_query(
            schema,
            '{ entity(id: "e1") { id content neighbors { id content } } }',
        )

        entity = data["entity"]
        assert entity is not None
        assert len(entity["neighbors"]) >= 1
        assert any("Related" in n["content"] for n in entity["neighbors"])


@pytest.mark.embedding
class TestMaintenanceDataQuery:
    async def test_maintenance_returns_health_and_stats(self, schema, setup_db_connection, db):
        """Test maintenance query returns health score and stats."""
        await seed_entity(db, "e1")
        await seed_entity(db, "e2")

        data = await execute_query(schema, """
            {
                maintenanceData {
                    health
                    stats { total conflicts stale }
                    conflicts { id }
                }
            }
        """)

        maint = data["maintenanceData"]
        assert maint["health"] >= 0
        assert maint["health"] <= 100
        assert maint["stats"]["total"] == "2"
        assert isinstance(maint["conflicts"], list)


# =============================================================================
# Mutation Tests
# =============================================================================


@pytest.mark.embedding
class TestSaveProcedureMutation:
    async def test_save_procedure_creates_new(self, schema, setup_db_connection, db):
        """Test saveProcedure creates a new procedure."""
        data = await execute_query(
            schema,
            """
            mutation {
                saveProcedure(procedure: {
                    name: "New Procedure"
                    description: "A test procedure"
                    steps: [
                        { content: "Step 1", optional: false }
                        { content: "Step 2", optional: true }
                    ]
                    labels: ["test"]
                    context: "test-project"
                }) {
                    id
                    name
                    description
                    steps { content optional }
                    labels
                    context
                }
            }
            """,
        )

        proc = data["saveProcedure"]
        assert proc["name"] == "New Procedure"
        assert proc["description"] == "A test procedure"
        assert len(proc["steps"]) == 2
        assert proc["labels"] == ["test"]
        assert proc["context"] == "test-project"

    async def test_save_procedure_updates_existing(self, schema, setup_db_connection, db):
        """Test saveProcedure updates an existing procedure."""
        # Create initial procedure
        await seed_procedure(db, "p1", "Original", "Original desc", [{"content": "Old step"}])

        # Update it
        data = await execute_query(
            schema,
            """
            mutation {
                saveProcedure(procedure: {
                    id: "p1"
                    name: "Updated"
                    description: "Updated desc"
                    steps: [{ content: "New step", optional: false }]
                }) {
                    id
                    name
                    description
                }
            }
            """,
        )

        proc = data["saveProcedure"]
        assert proc["name"] == "Updated"
        assert proc["description"] == "Updated desc"
