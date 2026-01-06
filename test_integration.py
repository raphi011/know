"""Integration tests for memcp query functions.

These tests require a running SurrealDB instance.
Run with: uv run pytest test_integration.py -v

To skip these tests when no database is available, use:
    uv run pytest test_integration.py -v -m "not integration"
"""

import asyncio
import os
import pytest
from datetime import datetime, timedelta

# Skip all tests if SURREALDB_URL is not set or SurrealDB is not available
pytestmark = pytest.mark.integration

# Check if SurrealDB is available
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")


@pytest.fixture(scope="module")
def event_loop():
    """Create event loop for async tests."""
    loop = asyncio.new_event_loop()
    yield loop
    loop.close()


@pytest.fixture(scope="module")
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
        # Initialize schema
        await db.query(SCHEMA_SQL)
        yield db
    except Exception as e:
        pytest.skip(f"SurrealDB not available: {e}")
    finally:
        try:
            await db.close()
        except Exception:
            pass


@pytest.fixture
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


@pytest.fixture(autouse=True)
async def cleanup_test_data(db_connection):
    """Clean up test data before and after each test."""
    # Clean up before test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE episode WHERE id CONTAINS 'test_'")
    yield
    # Clean up after test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE episode WHERE id CONTAINS 'test_'")


# =============================================================================
# Context Detection Tests
# =============================================================================

class TestContextDetection:
    def test_detect_context_explicit(self):
        """Test explicit context takes priority."""
        from memcp.db import detect_context
        result = detect_context("my-project")
        assert result == "my-project"

    def test_detect_context_none(self):
        """Test None returns None when no defaults set."""
        from memcp.db import detect_context
        import memcp.db as db_module

        # Save original values
        orig_default = db_module.MEMCP_DEFAULT_CONTEXT
        orig_cwd = db_module.MEMCP_CONTEXT_FROM_CWD

        try:
            db_module.MEMCP_DEFAULT_CONTEXT = None
            db_module.MEMCP_CONTEXT_FROM_CWD = False
            result = detect_context(None)
            assert result is None
        finally:
            # Restore original values
            db_module.MEMCP_DEFAULT_CONTEXT = orig_default
            db_module.MEMCP_CONTEXT_FROM_CWD = orig_cwd


# =============================================================================
# Entity Query Tests with Context and Importance
# =============================================================================

class TestEntityQueries:
    @pytest.mark.asyncio
    async def test_upsert_entity_with_context(self, mock_ctx):
        """Test entity creation with context."""
        from memcp.db import query_upsert_entity, query_get_entity
        from memcp.utils import embed

        embedding = embed("test content for context")
        await query_upsert_entity(
            mock_ctx,
            entity_id="test_context_entity",
            entity_type="test",
            labels=["test"],
            content="test content for context",
            embedding=embedding,
            confidence=1.0,
            source="test",
            context="test-project"
        )

        result = await query_get_entity(mock_ctx, "test_context_entity")
        assert len(result) == 1
        assert result[0].get('context') == "test-project"

    @pytest.mark.asyncio
    async def test_upsert_entity_with_importance(self, mock_ctx):
        """Test entity creation with user importance."""
        from memcp.db import query_upsert_entity, query_get_entity
        from memcp.utils import embed

        embedding = embed("important test content")
        await query_upsert_entity(
            mock_ctx,
            entity_id="test_importance_entity",
            entity_type="test",
            labels=["test"],
            content="important test content",
            embedding=embedding,
            confidence=1.0,
            source="test",
            context=None,
            user_importance=0.9
        )

        result = await query_get_entity(mock_ctx, "test_importance_entity")
        assert len(result) == 1
        assert result[0].get('user_importance') == 0.9

    @pytest.mark.asyncio
    async def test_hybrid_search_with_context(self, mock_ctx):
        """Test hybrid search filters by context."""
        from memcp.db import query_upsert_entity, query_hybrid_search
        from memcp.utils import embed

        # Create entities in different contexts
        embedding1 = embed("python programming language")
        await query_upsert_entity(
            mock_ctx, "test_py_proj1", "concept", ["programming"],
            "python programming language", embedding1, 1.0, "test", "project-a"
        )

        embedding2 = embed("python snake species")
        await query_upsert_entity(
            mock_ctx, "test_py_proj2", "concept", ["animals"],
            "python snake species", embedding2, 1.0, "test", "project-b"
        )

        # Search with context filter
        query_emb = embed("python")
        results = await query_hybrid_search(
            mock_ctx, "python", query_emb, [], 10, context="project-a"
        )

        # Should only find the programming entity
        assert len(results) == 1
        assert "programming" in results[0].get('content', '')


# =============================================================================
# Episode Query Tests
# =============================================================================

class TestEpisodeQueries:
    @pytest.mark.asyncio
    async def test_create_episode(self, mock_ctx):
        """Test episode creation."""
        from memcp.db import query_create_episode, query_get_episode
        from memcp.utils import embed

        content = "This is a test conversation episode"
        embedding = embed(content)
        timestamp = datetime.now().isoformat()

        await query_create_episode(
            mock_ctx,
            episode_id="test_episode_1",
            content=content,
            embedding=embedding,
            timestamp=timestamp,
            summary="Test episode",
            metadata={"source": "test"},
            context="test-project"
        )

        result = await query_get_episode(mock_ctx, "test_episode_1")
        assert len(result) == 1
        assert result[0].get('content') == content
        assert result[0].get('context') == "test-project"
        assert result[0].get('summary') == "Test episode"

    @pytest.mark.asyncio
    async def test_search_episodes(self, mock_ctx):
        """Test episode search."""
        from memcp.db import query_create_episode, query_search_episodes
        from memcp.utils import embed

        # Create test episodes
        for i in range(3):
            content = f"Test conversation {i} about machine learning"
            embedding = embed(content)
            await query_create_episode(
                mock_ctx,
                episode_id=f"test_search_ep_{i}",
                content=content,
                embedding=embedding,
                timestamp=datetime.now().isoformat(),
                summary=None,
                metadata={},
                context="ml-project"
            )

        # Search episodes
        query_emb = embed("machine learning")
        results = await query_search_episodes(
            mock_ctx, "machine learning", query_emb, None, None, None, 10
        )

        assert len(results) >= 3

    @pytest.mark.asyncio
    async def test_search_episodes_with_time_filter(self, mock_ctx):
        """Test episode search with time range filter."""
        from memcp.db import query_create_episode, query_search_episodes
        from memcp.utils import embed

        now = datetime.now()
        old_time = (now - timedelta(days=30)).isoformat()
        new_time = now.isoformat()

        # Create old episode
        old_content = "Old discussion about testing"
        await query_create_episode(
            mock_ctx, "test_old_ep", old_content, embed(old_content),
            old_time, None, {}, None
        )

        # Create new episode
        new_content = "New discussion about testing"
        await query_create_episode(
            mock_ctx, "test_new_ep", new_content, embed(new_content),
            new_time, None, {}, None
        )

        # Search with time filter (last 7 days)
        time_start = (now - timedelta(days=7)).isoformat()
        query_emb = embed("testing")
        results = await query_search_episodes(
            mock_ctx, "testing", query_emb, time_start, None, None, 10
        )

        # Should only find the new episode
        episode_ids = [str(r.get('id', '')) for r in results]
        assert any('test_new_ep' in eid for eid in episode_ids)

    @pytest.mark.asyncio
    async def test_link_entity_to_episode(self, mock_ctx):
        """Test linking entities to episodes."""
        from memcp.db import (
            query_create_episode, query_upsert_entity,
            query_link_entity_to_episode, query_get_episode_entities
        )
        from memcp.utils import embed

        # Create episode
        ep_content = "Discussion about Python programming"
        await query_create_episode(
            mock_ctx, "test_link_ep", ep_content, embed(ep_content),
            datetime.now().isoformat(), None, {}, None
        )

        # Create entity
        ent_content = "Python is a programming language"
        await query_upsert_entity(
            mock_ctx, "test_link_ent", "concept", ["python"],
            ent_content, embed(ent_content), 1.0, "test"
        )

        # Link entity to episode
        await query_link_entity_to_episode(
            mock_ctx, "test_link_ent", "test_link_ep", position=0, confidence=1.0
        )

        # Get episode entities
        result = await query_get_episode_entities(mock_ctx, "test_link_ep")
        assert len(result) > 0


# =============================================================================
# Importance Scoring Tests
# =============================================================================

class TestImportanceScoring:
    @pytest.mark.asyncio
    async def test_recalculate_importance(self, mock_ctx):
        """Test importance recalculation."""
        from memcp.db import query_upsert_entity, query_recalculate_importance, query_get_entity
        from memcp.utils import embed

        # Create entity
        content = "Test entity for importance calculation"
        await query_upsert_entity(
            mock_ctx, "test_imp_entity", "test", ["test"],
            content, embed(content), 1.0, "test",
            user_importance=0.8
        )

        # Recalculate importance
        importance = await query_recalculate_importance(mock_ctx, "test_imp_entity")

        # Importance should be calculated
        assert 0 <= importance <= 1

        # Check entity was updated
        result = await query_get_entity(mock_ctx, "test_imp_entity")
        assert result[0].get('importance') is not None

    @pytest.mark.asyncio
    async def test_importance_with_relations(self, mock_ctx):
        """Test importance increases with more relations."""
        from memcp.db import (
            query_upsert_entity, query_create_relation,
            query_recalculate_importance, query_get_entity
        )
        from memcp.utils import embed

        # Create main entity
        main_content = "Main entity with relations"
        await query_upsert_entity(
            mock_ctx, "test_main_ent", "test", [],
            main_content, embed(main_content), 1.0, "test"
        )

        # Get importance before relations
        importance_before = await query_recalculate_importance(mock_ctx, "test_main_ent")

        # Create related entities and relations
        for i in range(5):
            rel_content = f"Related entity {i}"
            await query_upsert_entity(
                mock_ctx, f"test_rel_ent_{i}", "test", [],
                rel_content, embed(rel_content), 1.0, "test"
            )
            await query_create_relation(
                mock_ctx, "test_main_ent", "related_to", f"test_rel_ent_{i}", 1.0
            )

        # Recalculate importance after relations
        importance_after = await query_recalculate_importance(mock_ctx, "test_main_ent")

        # Importance should be higher with more connections
        assert importance_after >= importance_before


# =============================================================================
# Context Management Tests
# =============================================================================

class TestContextManagement:
    @pytest.mark.asyncio
    async def test_list_contexts(self, mock_ctx):
        """Test listing all contexts."""
        from memcp.db import query_upsert_entity, query_list_contexts
        from memcp.utils import embed

        # Create entities in different contexts
        contexts = ["test-project-a", "test-project-b", "test-project-c"]
        for ctx_name in contexts:
            content = f"Entity in {ctx_name}"
            await query_upsert_entity(
                mock_ctx, f"test_ctx_{ctx_name}", "test", [],
                content, embed(content), 1.0, "test", context=ctx_name
            )

        # List contexts
        result = await query_list_contexts(mock_ctx)
        found_contexts = result[0].get('contexts', [])

        # All test contexts should be found
        for ctx_name in contexts:
            assert ctx_name in found_contexts

    @pytest.mark.asyncio
    async def test_get_context_stats(self, mock_ctx):
        """Test getting context statistics."""
        from memcp.db import (
            query_upsert_entity, query_create_episode,
            query_get_context_stats
        )
        from memcp.utils import embed

        ctx_name = "test-stats-project"

        # Create entities in context
        for i in range(3):
            content = f"Entity {i} in stats project"
            await query_upsert_entity(
                mock_ctx, f"test_stats_ent_{i}", "test", [],
                content, embed(content), 1.0, "test", context=ctx_name
            )

        # Create episodes in context
        for i in range(2):
            content = f"Episode {i} in stats project"
            await query_create_episode(
                mock_ctx, f"test_stats_ep_{i}", content, embed(content),
                datetime.now().isoformat(), None, {}, ctx_name
            )

        # Get stats
        result = await query_get_context_stats(mock_ctx, ctx_name)
        stats = result[0] if result else {}

        assert stats.get('context') == ctx_name
        assert stats.get('entities', 0) >= 3
        assert stats.get('episodes', 0) >= 2


# =============================================================================
# Model Tests
# =============================================================================

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

    def test_episode_result(self):
        """Test EpisodeResult model."""
        from memcp.models import EpisodeResult

        episode = EpisodeResult(
            id="episode:123",
            content="Test episode content",
            timestamp="2024-01-01T00:00:00",
            summary="Test summary",
            metadata={"key": "value"},
            context="project",
            linked_entities=5
        )

        assert episode.id == "episode:123"
        assert episode.linked_entities == 5

    def test_context_stats(self):
        """Test ContextStats model."""
        from memcp.models import ContextStats

        stats = ContextStats(
            context="my-project",
            entities=10,
            episodes=5,
            relations=20
        )

        assert stats.context == "my-project"
        assert stats.entities == 10

    def test_reflect_result_with_importance(self):
        """Test ReflectResult includes importance_recalculated."""
        from memcp.models import ReflectResult

        result = ReflectResult(
            decayed=5,
            merged=2,
            importance_recalculated=100
        )

        assert result.importance_recalculated == 100
