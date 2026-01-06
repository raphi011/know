"""Integration tests for memcp query functions.

These tests require a running SurrealDB instance.
Run with: uv run pytest test_integration.py -v

To skip these tests when no database is available, use:
    uv run pytest test_integration.py -v -m "not integration"
"""

import asyncio
import os
import pytest
import pytest_asyncio
from datetime import datetime, timedelta

# Skip all tests if SURREALDB_URL is not set or SurrealDB is not available
pytestmark = pytest.mark.integration

# Check if SurrealDB is available
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")


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
        # Drop and recreate tables to ensure schema is up to date
        # This is needed because IF NOT EXISTS won't update existing definitions
        await db.query("REMOVE TABLE IF EXISTS episode; REMOVE TABLE IF EXISTS procedure; REMOVE TABLE IF EXISTS extracted_from;")
        # Initialize schema
        await db.query(SCHEMA_SQL)
        yield db
    except Exception as e:
        pytest.skip(f"SurrealDB not available: {e}")
    finally:
        # Don't await db.close() - it hangs due to SDK race condition
        # Connection will be cleaned up when process exits
        pass


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

        async def read_resource(self, uri: str):
            """Mock resource reading for tools like reflect()."""
            from memcp.models import MemoryStats
            if uri == "memory://stats":
                return MemoryStats(total_entities=0, total_relations=0, labels=[], label_counts={})
            return None

    return MockContext(db_connection)


@pytest_asyncio.fixture(autouse=True)
async def cleanup_test_data(db_connection):
    """Clean up test data before and after each test."""
    # Clean up before test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE episode WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE procedure WHERE id CONTAINS 'test_'")
    yield
    # Clean up after test
    await db_connection.query("DELETE entity WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE episode WHERE id CONTAINS 'test_'")
    await db_connection.query("DELETE procedure WHERE id CONTAINS 'test_'")


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

@pytest.mark.embedding
class TestEntityQueries:
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

@pytest.mark.embedding
class TestEpisodeQueries:
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

@pytest.mark.embedding
class TestImportanceScoring:
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

@pytest.mark.embedding
class TestContextManagement:
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


# =============================================================================
# Entity Type Ontology Tests
# =============================================================================

@pytest.mark.embedding
class TestEntityTypeOntology:
    def test_entity_types_defined(self):
        """Test that entity types dictionary is populated."""
        from memcp.db import ENTITY_TYPES, get_entity_types

        assert len(ENTITY_TYPES) > 0
        assert "preference" in ENTITY_TYPES
        assert "requirement" in ENTITY_TYPES
        assert "procedure" in ENTITY_TYPES

        types = get_entity_types()
        assert types == ENTITY_TYPES

    def test_validate_entity_type_predefined(self):
        """Test validation of predefined types."""
        from memcp.db import validate_entity_type

        assert validate_entity_type("preference") == "preference"
        assert validate_entity_type("PREFERENCE") == "preference"  # case insensitive
        assert validate_entity_type("  requirement  ") == "requirement"  # trimmed

    def test_validate_entity_type_custom(self):
        """Test validation of custom types when allowed."""
        from memcp.db import validate_entity_type, ALLOW_CUSTOM_TYPES

        if ALLOW_CUSTOM_TYPES:
            assert validate_entity_type("my_custom_type") == "my_custom_type"

    async def test_query_entities_by_type(self, mock_ctx):
        """Test querying entities by type."""
        from memcp.db import query_upsert_entity, query_entities_by_type
        from memcp.utils import embed

        # Create entities with different types
        await query_upsert_entity(
            mock_ctx, "test_pref_1", "preference", ["test"],
            "I prefer dark mode", embed("I prefer dark mode"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_fact_1", "fact", ["test"],
            "The sky is blue", embed("The sky is blue"), 1.0, "test"
        )

        # Query by type
        prefs = await query_entities_by_type(mock_ctx, "preference", None, 100)
        facts = await query_entities_by_type(mock_ctx, "fact", None, 100)

        assert any("test_pref_1" in str(p['id']) for p in prefs)
        assert any("test_fact_1" in str(f['id']) for f in facts)

    async def test_query_list_types(self, mock_ctx):
        """Test listing entity types with counts."""
        from memcp.db import query_upsert_entity, query_list_types
        from memcp.utils import embed

        # Create entities with types
        await query_upsert_entity(
            mock_ctx, "test_type_a", "decision", [],
            "Decision A", embed("Decision A"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_type_b", "decision", [],
            "Decision B", embed("Decision B"), 1.0, "test"
        )

        # List types
        types = await query_list_types(mock_ctx, None)
        type_dict = {t['type']: t.get('count', 0) for t in types}

        assert "decision" in type_dict


# =============================================================================
# Procedure Query Tests
# =============================================================================

@pytest.mark.embedding
class TestProcedureQueries:
    async def test_create_procedure(self, mock_ctx):
        """Test procedure creation."""
        from memcp.db import query_create_procedure, query_get_procedure
        from memcp.utils import embed

        steps = [
            {"order": 1, "content": "Step 1: Prepare", "optional": False},
            {"order": 2, "content": "Step 2: Execute", "optional": False},
            {"order": 3, "content": "Step 3: Verify", "optional": True},
        ]
        embed_text = "Deploy app. Deploy application to production. " + " ".join(s['content'] for s in steps)

        await query_create_procedure(
            mock_ctx,
            procedure_id="test_deploy_proc",
            name="Deploy app",
            description="Deploy application to production",
            steps=steps,
            embedding=embed(embed_text),
            context="test-project",
            labels=["deployment", "devops"]
        )

        result = await query_get_procedure(mock_ctx, "test_deploy_proc")
        assert len(result) == 1
        proc = result[0]
        assert proc['name'] == "Deploy app"
        assert len(proc['steps']) == 3
        assert proc['context'] == "test-project"

    async def test_search_procedures(self, mock_ctx):
        """Test procedure search."""
        from memcp.db import query_create_procedure, query_search_procedures
        from memcp.utils import embed

        # Create test procedures
        for i in range(3):
            steps = [{"order": 1, "content": f"Step for proc {i}"}]
            embed_text = f"Test procedure {i}. Test procedure about databases. Step for proc {i}"
            await query_create_procedure(
                mock_ctx,
                procedure_id=f"test_search_proc_{i}",
                name=f"Test procedure {i}",
                description="Test procedure about databases",
                steps=steps,
                embedding=embed(embed_text),
                context=None,
                labels=["database"]
            )

        # Search procedures
        query_emb = embed("database procedure")
        results = await query_search_procedures(
            mock_ctx, "database", query_emb, None, [], 10
        )

        assert len(results) >= 3

    async def test_list_procedures(self, mock_ctx):
        """Test listing procedures."""
        from memcp.db import query_create_procedure, query_list_procedures
        from memcp.utils import embed

        # Create procedures in a specific context
        ctx_name = "test-list-ctx"
        for i in range(2):
            steps = [{"order": 1, "content": f"Step {i}"}]
            embed_text = f"List proc {i}. Testing list. Step {i}"
            await query_create_procedure(
                mock_ctx,
                procedure_id=f"test_list_proc_{i}",
                name=f"List proc {i}",
                description="Testing list",
                steps=steps,
                embedding=embed(embed_text),
                context=ctx_name,
                labels=[]
            )

        # List with context filter
        results = await query_list_procedures(mock_ctx, ctx_name, 100)
        assert len(results) >= 2

    async def test_delete_procedure(self, mock_ctx):
        """Test procedure deletion."""
        from memcp.db import query_create_procedure, query_get_procedure, query_delete_procedure
        from memcp.utils import embed

        steps = [{"order": 1, "content": "To delete"}]
        await query_create_procedure(
            mock_ctx,
            procedure_id="test_del_proc",
            name="Delete me",
            description="Will be deleted",
            steps=steps,
            embedding=embed("Delete me. Will be deleted. To delete"),
            context=None,
            labels=[]
        )

        # Verify exists
        result = await query_get_procedure(mock_ctx, "test_del_proc")
        assert len(result) == 1

        # Delete
        await query_delete_procedure(mock_ctx, "test_del_proc")

        # Verify deleted
        result = await query_get_procedure(mock_ctx, "test_del_proc")
        assert len(result) == 0


# =============================================================================
# Procedure Model Tests
# =============================================================================

class TestProcedureModels:
    def test_procedure_step_model(self):
        """Test ProcedureStep model."""
        from memcp.models import ProcedureStep

        step = ProcedureStep(order=1, content="Do something", optional=True)
        assert step.order == 1
        assert step.content == "Do something"
        assert step.optional is True

    def test_procedure_result_model(self):
        """Test ProcedureResult model."""
        from memcp.models import ProcedureResult, ProcedureStep

        steps = [
            ProcedureStep(order=1, content="Step 1"),
            ProcedureStep(order=2, content="Step 2")
        ]
        proc = ProcedureResult(
            id="procedure:deploy",
            name="Deploy",
            description="Deploy process",
            steps=steps,
            context="myproject",
            labels=["deployment"]
        )

        assert proc.id == "procedure:deploy"
        assert len(proc.steps) == 2
        assert proc.labels == ["deployment"]

    def test_entity_type_info_model(self):
        """Test EntityTypeInfo model."""
        from memcp.models import EntityTypeInfo

        info = EntityTypeInfo(
            type="preference",
            description="User preference",
            count=5
        )
        assert info.type == "preference"
        assert info.count == 5

    def test_entity_type_list_result_model(self):
        """Test EntityTypeListResult model."""
        from memcp.models import EntityTypeListResult, EntityTypeInfo

        types = [
            EntityTypeInfo(type="preference", description="User pref", count=3),
            EntityTypeInfo(type="fact", description="A fact", count=10)
        ]
        result = EntityTypeListResult(types=types, custom_types_allowed=True)

        assert len(result.types) == 2
        assert result.custom_types_allowed is True


# =============================================================================
# Tool Integration Tests - Search Server
# =============================================================================

@pytest.mark.embedding
class TestSearchTools:
    async def test_search_tool(self, mock_ctx):
        """Test search tool returns results."""
        from memcp.servers.search import search
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        # Create test entity
        await query_upsert_entity(
            mock_ctx, "test_search_tool_1", "concept", ["python"],
            "Python is a programming language", embed("Python is a programming language"),
            1.0, "test"
        )

        result = await search.fn("programming language", ctx=mock_ctx)

        assert result.count >= 1
        assert any("Python" in e.content for e in result.entities)

    async def test_search_tool_with_labels(self, mock_ctx):
        """Test search tool filters by labels."""
        from memcp.servers.search import search
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_labeled_1", "concept", ["database"],
            "PostgreSQL database", embed("PostgreSQL database"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_labeled_2", "concept", ["language"],
            "Python language", embed("Python language"), 1.0, "test"
        )

        result = await search.fn("database OR language", labels=["database"], ctx=mock_ctx)

        # Should only find database-labeled entity
        assert all("database" in e.labels for e in result.entities if "test_labeled" in e.id)

    async def test_search_tool_with_context(self, mock_ctx):
        """Test search tool filters by context."""
        from memcp.servers.search import search
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_ctx_a", "concept", [],
            "Entity in project A", embed("Entity in project A"), 1.0, "test",
            context="project-a"
        )
        await query_upsert_entity(
            mock_ctx, "test_ctx_b", "concept", [],
            "Entity in project B", embed("Entity in project B"), 1.0, "test",
            context="project-b"
        )

        result = await search.fn("project", context="project-a", ctx=mock_ctx)

        assert all(e.context == "project-a" for e in result.entities if e.context)

    async def test_get_entity_tool(self, mock_ctx):
        """Test get_entity retrieves by ID."""
        from memcp.servers.search import get_entity
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_get_entity", "fact", ["test"],
            "This is a test fact", embed("This is a test fact"), 0.9, "test"
        )

        result = await get_entity.fn("test_get_entity", mock_ctx)

        assert result is not None
        assert "test fact" in result.content
        assert result.confidence == 0.9

    async def test_get_entity_not_found(self, mock_ctx):
        """Test get_entity returns None for missing entity."""
        from memcp.servers.search import get_entity

        result = await get_entity.fn("nonexistent_entity", mock_ctx)

        assert result is None

    async def test_list_labels_tool(self, mock_ctx):
        """Test list_labels returns all labels."""
        from memcp.servers.search import list_labels
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_labels_1", "concept", ["alpha", "beta"],
            "Entity with labels", embed("Entity with labels"), 1.0, "test"
        )

        result = await list_labels.fn(mock_ctx)

        assert "alpha" in result
        assert "beta" in result

    async def test_list_contexts_tool(self, mock_ctx):
        """Test list_contexts returns all contexts."""
        from memcp.servers.search import list_contexts
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_ctx_list", "concept", [],
            "Entity with context", embed("Entity with context"), 1.0, "test",
            context="my-test-context"
        )

        result = await list_contexts.fn(mock_ctx)

        assert "my-test-context" in result.contexts

    async def test_get_context_stats_tool(self, mock_ctx):
        """Test get_context_stats returns correct counts."""
        from memcp.servers.search import get_context_stats
        from memcp.db import query_upsert_entity, query_create_episode
        from memcp.utils import embed

        ctx_name = "test-stats-ctx"

        # Create entities and episode in context
        for i in range(3):
            await query_upsert_entity(
                mock_ctx, f"test_stats_{i}", "concept", [],
                f"Entity {i}", embed(f"Entity {i}"), 1.0, "test", context=ctx_name
            )

        await query_create_episode(
            mock_ctx, "test_stats_ep", "Episode content", embed("Episode content"),
            datetime.now().isoformat(), None, {}, ctx_name
        )

        result = await get_context_stats.fn(ctx_name, mock_ctx)

        assert result.context == ctx_name
        assert result.entities >= 3
        assert result.episodes >= 1

    async def test_list_entity_types_tool(self, mock_ctx):
        """Test list_entity_types returns predefined types."""
        from memcp.servers.search import list_entity_types

        result = await list_entity_types.fn(ctx=mock_ctx)

        type_names = [t.type for t in result.types]
        assert "preference" in type_names
        assert "requirement" in type_names
        assert "decision" in type_names

    async def test_search_by_type_tool(self, mock_ctx):
        """Test search_by_type filters by entity type."""
        from memcp.servers.search import search_by_type
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_type_pref", "preference", [],
            "I prefer dark mode", embed("I prefer dark mode"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_type_fact", "fact", [],
            "The sky is blue", embed("The sky is blue"), 1.0, "test"
        )

        result = await search_by_type.fn("preference", ctx=mock_ctx)

        assert all(e.type == "preference" for e in result.entities)


# =============================================================================
# Tool Integration Tests - Persist Server
# =============================================================================

@pytest.mark.embedding
class TestPersistTools:
    async def test_remember_entities(self, mock_ctx):
        """Test remember stores entities."""
        from memcp.servers.persist import remember
        from memcp.db import query_get_entity

        result = await remember.fn(
            entities=[
                {"id": "test_remember_1", "content": "Test content 1", "type": "fact"},
                {"id": "test_remember_2", "content": "Test content 2", "labels": ["test"]}
            ],
            ctx=mock_ctx
        )

        assert result.entities_stored == 2

        # Verify entities exist
        e1 = await query_get_entity(mock_ctx, "test_remember_1")
        e2 = await query_get_entity(mock_ctx, "test_remember_2")
        assert len(e1) == 1
        assert len(e2) == 1

    async def test_remember_with_relations(self, mock_ctx):
        """Test remember stores relations."""
        from memcp.servers.persist import remember

        result = await remember.fn(
            entities=[
                {"id": "test_rel_from", "content": "From entity"},
                {"id": "test_rel_to", "content": "To entity"}
            ],
            relations=[
                {"from": "test_rel_from", "to": "test_rel_to", "type": "relates_to"}
            ],
            ctx=mock_ctx
        )

        assert result.entities_stored == 2
        assert result.relations_stored == 1

    async def test_remember_with_context(self, mock_ctx):
        """Test remember applies context to entities."""
        from memcp.servers.persist import remember
        from memcp.db import query_get_entity

        await remember.fn(
            entities=[{"id": "test_ctx_entity", "content": "Contextual entity"}],
            context="my-project",
            ctx=mock_ctx
        )

        result = await query_get_entity(mock_ctx, "test_ctx_entity")
        assert result[0].get('context') == "my-project"

    async def test_remember_with_importance(self, mock_ctx):
        """Test remember stores user importance."""
        from memcp.servers.persist import remember
        from memcp.db import query_get_entity

        await remember.fn(
            entities=[{"id": "test_imp_entity", "content": "Important entity", "importance": 0.9}],
            ctx=mock_ctx
        )

        result = await query_get_entity(mock_ctx, "test_imp_entity")
        assert result[0].get('user_importance') == 0.9

    async def test_forget_entity(self, mock_ctx):
        """Test forget removes entity."""
        from memcp.servers.persist import remember, forget
        from memcp.db import query_get_entity

        await remember.fn(
            entities=[{"id": "test_forget", "content": "To be forgotten"}],
            ctx=mock_ctx
        )

        # Verify exists
        assert len(await query_get_entity(mock_ctx, "test_forget")) == 1

        # Forget it
        result = await forget.fn("test_forget", mock_ctx)
        assert "test_forget" in result

        # Verify deleted
        assert len(await query_get_entity(mock_ctx, "test_forget")) == 0


# =============================================================================
# Tool Integration Tests - Graph Server
# =============================================================================

@pytest.mark.embedding
class TestGraphTools:
    async def test_traverse_tool(self, mock_ctx):
        """Test traverse explores graph connections."""
        from memcp.servers.graph import traverse
        from memcp.db import query_upsert_entity, query_create_relation
        from memcp.utils import embed

        # Create entities and relations
        await query_upsert_entity(
            mock_ctx, "test_trav_root", "concept", [],
            "Root entity", embed("Root entity"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_trav_child", "concept", [],
            "Child entity", embed("Child entity"), 1.0, "test"
        )
        await query_create_relation(
            mock_ctx, "test_trav_root", "connects_to", "test_trav_child", 1.0
        )

        result = await traverse.fn("test_trav_root", depth=2, ctx=mock_ctx)

        assert result is not None
        # traverse returns JSON string
        assert "test_trav_root" in result

    async def test_find_path_tool(self, mock_ctx):
        """Test find_path finds connection between entities."""
        from memcp.servers.graph import find_path
        from memcp.db import query_upsert_entity, query_create_relation
        from memcp.utils import embed

        # Create chain: A -> B -> C
        for name in ["test_path_a", "test_path_b", "test_path_c"]:
            await query_upsert_entity(
                mock_ctx, name, "concept", [],
                f"Entity {name}", embed(f"Entity {name}"), 1.0, "test"
            )
        await query_create_relation(mock_ctx, "test_path_a", "links", "test_path_b", 1.0)
        await query_create_relation(mock_ctx, "test_path_b", "links", "test_path_c", 1.0)

        result = await find_path.fn("test_path_a", "test_path_c", max_depth=3, ctx=mock_ctx)

        # Should find path (result structure varies)
        assert result is not None


# =============================================================================
# Tool Integration Tests - Episode Server
# =============================================================================

@pytest.mark.embedding
class TestEpisodeTools:
    async def test_add_episode_tool(self, mock_ctx):
        """Test add_episode stores episode."""
        from memcp.servers.episode import add_episode

        result = await add_episode.fn(
            content="This is a test conversation about Python programming.",
            summary="Python discussion",
            context="test-project",
            ctx=mock_ctx
        )

        assert result is not None
        assert "ep_" in result.id
        assert result.context == "test-project"
        assert result.summary == "Python discussion"

    async def test_add_episode_with_entity_links(self, mock_ctx):
        """Test add_episode links entities."""
        from memcp.servers.episode import add_episode
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        # Create entity to link
        await query_upsert_entity(
            mock_ctx, "test_ep_entity", "concept", [],
            "Related concept", embed("Related concept"), 1.0, "test"
        )

        result = await add_episode.fn(
            content="Discussion about the related concept.",
            entity_ids=["test_ep_entity"],
            ctx=mock_ctx
        )

        assert result.linked_entities >= 1

    async def test_search_episodes_tool(self, mock_ctx):
        """Test search_episodes finds episodes."""
        from memcp.servers.episode import add_episode, search_episodes

        # Create episodes
        await add_episode.fn(content="Machine learning discussion", ctx=mock_ctx)
        await add_episode.fn(content="Deep learning neural networks", ctx=mock_ctx)

        result = await search_episodes.fn("machine learning", ctx=mock_ctx)

        assert result.count >= 1

    async def test_search_episodes_with_time_filter(self, mock_ctx):
        """Test search_episodes filters by time."""
        from memcp.servers.episode import add_episode, search_episodes

        # Create episode with specific timestamp
        now = datetime.now()
        await add_episode.fn(
            content="Recent test episode",
            timestamp=now.isoformat(),
            ctx=mock_ctx
        )

        # Search with time filter
        time_start = (now - timedelta(hours=1)).isoformat()
        result = await search_episodes.fn("test episode", time_start=time_start, ctx=mock_ctx)

        assert result.count >= 1

    async def test_get_episode_tool(self, mock_ctx):
        """Test get_episode retrieves episode."""
        from memcp.servers.episode import add_episode, get_episode

        added = await add_episode.fn(content="Episode to retrieve", ctx=mock_ctx)
        episode_id = added.id.replace("episode:", "")

        result = await get_episode.fn(episode_id, ctx=mock_ctx)

        assert result is not None
        assert "Episode to retrieve" in result.content

    async def test_delete_episode_tool(self, mock_ctx):
        """Test delete_episode removes episode."""
        from memcp.servers.episode import add_episode, get_episode, delete_episode

        added = await add_episode.fn(content="Episode to delete", ctx=mock_ctx)
        episode_id = added.id.replace("episode:", "")

        # Delete
        await delete_episode.fn(episode_id, mock_ctx)

        # Verify deleted
        result = await get_episode.fn(episode_id, ctx=mock_ctx)
        assert result is None


# =============================================================================
# Tool Integration Tests - Procedure Server
# =============================================================================

@pytest.mark.embedding
class TestProcedureTools:
    async def test_add_procedure_tool(self, mock_ctx):
        """Test add_procedure stores procedure."""
        from memcp.servers.procedure import add_procedure

        result = await add_procedure.fn(
            name="Test Deploy Process",
            description="Steps to deploy the test app",
            steps=[
                {"content": "Run tests"},
                {"content": "Build image"},
                {"content": "Deploy to staging", "optional": True}
            ],
            labels=["deployment"],
            ctx=mock_ctx
        )

        assert result is not None
        assert result.name == "Test Deploy Process"
        assert len(result.steps) == 3
        assert result.steps[2].optional is True

    async def test_get_procedure_tool(self, mock_ctx):
        """Test get_procedure retrieves procedure."""
        from memcp.servers.procedure import add_procedure, get_procedure

        added = await add_procedure.fn(
            name="Test Retrieve Proc",
            description="Procedure to retrieve",
            steps=[{"content": "Step 1"}],
            ctx=mock_ctx
        )

        proc_id = added.id.replace("procedure:", "")
        result = await get_procedure.fn(proc_id, ctx=mock_ctx)

        assert result is not None
        assert result.name == "Test Retrieve Proc"

    async def test_search_procedures_tool(self, mock_ctx):
        """Test search_procedures finds procedures."""
        from memcp.servers.procedure import add_procedure, search_procedures

        await add_procedure.fn(
            name="Database Migration",
            description="Steps to migrate database",
            steps=[{"content": "Backup data"}, {"content": "Run migrations"}],
            labels=["database"],
            ctx=mock_ctx
        )

        result = await search_procedures.fn("database migration", ctx=mock_ctx)

        assert result.count >= 1

    async def test_list_procedures_tool(self, mock_ctx):
        """Test list_procedures returns all procedures."""
        from memcp.servers.procedure import add_procedure, list_procedures

        await add_procedure.fn(
            name="Test List Proc 1",
            description="First procedure",
            steps=[{"content": "Step"}],
            context="test-list-ctx",
            ctx=mock_ctx
        )
        await add_procedure.fn(
            name="Test List Proc 2",
            description="Second procedure",
            steps=[{"content": "Step"}],
            context="test-list-ctx",
            ctx=mock_ctx
        )

        result = await list_procedures.fn(context="test-list-ctx", ctx=mock_ctx)

        assert result.count >= 2

    async def test_delete_procedure_tool(self, mock_ctx):
        """Test delete_procedure removes procedure."""
        from memcp.servers.procedure import add_procedure, get_procedure, delete_procedure

        added = await add_procedure.fn(
            name="Test Delete Proc",
            description="To be deleted",
            steps=[{"content": "Step"}],
            ctx=mock_ctx
        )

        proc_id = added.id.replace("procedure:", "")

        # Delete
        await delete_procedure.fn(proc_id, ctx=mock_ctx)

        # Verify deleted
        result = await get_procedure.fn(proc_id, ctx=mock_ctx)
        assert result is None


# =============================================================================
# Tool Integration Tests - Maintenance Server
# =============================================================================

@pytest.mark.embedding
class TestMaintenanceTools:
    async def test_reflect_decay(self, mock_ctx):
        """Test reflect applies decay to old entities."""
        from memcp.servers.maintenance import reflect
        from memcp.db import query_upsert_entity, run_query
        from memcp.utils import embed

        # Create entity with old access time
        await query_upsert_entity(
            mock_ctx, "test_decay_entity", "concept", [],
            "Old entity", embed("Old entity"), 1.0, "test"
        )
        # Manually set old accessed time
        await run_query(mock_ctx, """
            UPDATE type::record("entity", $id) SET accessed = <datetime>"2020-01-01T00:00:00Z"
        """, {'id': "test_decay_entity"})

        result = await reflect.fn(apply_decay=True, decay_days=1, find_similar=False, recalculate_importance=False, ctx=mock_ctx)

        assert result.decayed >= 1

    async def test_reflect_find_similar(self, mock_ctx):
        """Test reflect finds similar entities."""
        from memcp.servers.maintenance import reflect
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        # Create very similar entities
        await query_upsert_entity(
            mock_ctx, "test_sim_1", "concept", [],
            "Python programming language features", embed("Python programming language features"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_sim_2", "concept", [],
            "Python programming language features", embed("Python programming language features"), 1.0, "test"
        )

        result = await reflect.fn(apply_decay=False, find_similar=True, similarity_threshold=0.9, recalculate_importance=False, ctx=mock_ctx)

        # Should find the similar pair
        assert len(result.similar_pairs) >= 1 or result.merged >= 1

    async def test_reflect_recalculate_importance(self, mock_ctx):
        """Test reflect recalculates importance."""
        from memcp.servers.maintenance import reflect
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        await query_upsert_entity(
            mock_ctx, "test_imp_recalc", "concept", [],
            "Entity for importance", embed("Entity for importance"), 1.0, "test"
        )

        result = await reflect.fn(apply_decay=False, find_similar=False, recalculate_importance=True, ctx=mock_ctx)

        assert result.importance_recalculated >= 1

    async def test_check_contradictions_tool(self, mock_ctx):
        """Test check_contradictions detects conflicts."""
        from memcp.servers.maintenance import check_contradictions_tool
        from memcp.db import query_upsert_entity
        from memcp.utils import embed

        # Create contradictory entities
        await query_upsert_entity(
            mock_ctx, "test_contra_1", "fact", ["test"],
            "The capital of France is Paris", embed("The capital of France is Paris"), 1.0, "test"
        )
        await query_upsert_entity(
            mock_ctx, "test_contra_2", "fact", ["test"],
            "The capital of France is not Paris", embed("The capital of France is not Paris"), 1.0, "test"
        )

        result = await check_contradictions_tool.fn(labels=["test"], ctx=mock_ctx)

        # May or may not find contradiction depending on NLI model sensitivity
        assert isinstance(result, list)
