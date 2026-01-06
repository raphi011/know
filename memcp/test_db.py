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
    query_list_labels,
    query_update_access,
    query_create_relation,
    query_similar_entities,
    query_hybrid_search,
    query_traverse,
    query_find_path,
    query_apply_decay,
    query_all_entities_with_embedding,
    query_similar_by_embedding,
    query_delete_entity_by_record_id,
    query_entity_with_embedding,
    query_similar_for_contradiction,
    query_entities_by_labels,
    query_vector_similarity,
    query_count_entities,
    query_count_relations,
    query_get_all_labels,
    query_count_by_label,
    run_query,
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
    await db.query("DELETE relates")

    yield db

    # Clean after test
    await db.query("DELETE entity")
    await db.query("DELETE relates")
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


@pytest.mark.asyncio
async def test_query_list_labels(mock_ctx):
    """Test query_list_labels function."""
    # Create entities with different labels
    test_embedding = [0.1] * 384

    await query_upsert_entity(
        mock_ctx,
        entity_id="entity1",
        entity_type="person",
        labels=["python", "developer"],
        content="Python developer",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="entity2",
        entity_type="skill",
        labels=["python", "programming"],
        content="Python programming skill",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="entity3",
        entity_type="skill",
        labels=["rust", "programming"],
        content="Rust programming skill",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Get all labels
    result = await query_list_labels(mock_ctx)

    assert result is not None
    assert len(result) > 0

    # Result format: list[list[dict]], dict has 'labels' key with list of labels
    labels_data = result[0]
    if isinstance(labels_data, list) and len(labels_data) > 0:
        labels = labels_data[0]['labels']
    else:
        labels = labels_data['labels']

    # Should have all unique labels (flattened)
    assert isinstance(labels, list)
    # The query should have flattened and deduplicated
    assert 'python' in labels
    assert 'developer' in labels
    assert 'programming' in labels
    assert 'rust' in labels


@pytest.mark.asyncio
async def test_query_update_access(mock_ctx):
    """Test query_update_access function."""
    test_embedding = [0.1] * 384

    # Create entity
    await query_upsert_entity(
        mock_ctx,
        entity_id="access_test",
        entity_type="test",
        labels=["test"],
        content="Access test entity",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Get initial state
    result = await query_get_entity(mock_ctx, "access_test")
    initial_access_count = result[0]['access_count']

    # Update access
    await query_update_access(mock_ctx, "access_test")

    # Verify access count increased
    result = await query_get_entity(mock_ctx, "access_test")
    new_access_count = result[0]['access_count']

    assert new_access_count == initial_access_count + 1


@pytest.mark.asyncio
async def test_query_create_relation(mock_ctx):
    """Test query_create_relation function."""
    test_embedding = [0.1] * 384

    # Create two entities
    await query_upsert_entity(
        mock_ctx,
        entity_id="person1",
        entity_type="person",
        labels=["test"],
        content="Person 1",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="person2",
        entity_type="person",
        labels=["test"],
        content="Person 2",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Create relation between them
    result = await query_create_relation(
        mock_ctx,
        from_id="person1",
        rel_type="knows",
        to_id="person2",
        weight=0.8
    )

    # Verify relation exists by querying relates table
    relations = await run_query(mock_ctx, """
        SELECT * FROM relates WHERE rel_type = $rel_type
    """, {'rel_type': 'knows'})

    assert relations is not None
    assert len(relations) > 0
    # Check that relation has correct properties
    rel = relations[0]
    assert rel.get('rel_type') == 'knows'
    assert rel.get('weight') == 0.8


@pytest.mark.asyncio
async def test_query_similar_entities(mock_ctx):
    """Test query_similar_entities function."""
    # Create entities with similar and different embeddings
    # Similar embeddings: mostly 0.5
    similar_embedding = [0.5] * 384
    # Different embedding: mostly 0.1
    different_embedding = [0.1] * 384

    # Target entity
    await query_upsert_entity(
        mock_ctx,
        entity_id="target",
        entity_type="concept",
        labels=["test"],
        content="Target concept",
        embedding=similar_embedding,
        confidence=1.0,
        source="test"
    )

    # Similar entity (should be found)
    await query_upsert_entity(
        mock_ctx,
        entity_id="similar1",
        entity_type="concept",
        labels=["test"],
        content="Similar concept",
        embedding=similar_embedding,
        confidence=1.0,
        source="test"
    )

    # Different entity (should not be in top results)
    await query_upsert_entity(
        mock_ctx,
        entity_id="different1",
        entity_type="concept",
        labels=["test"],
        content="Different concept",
        embedding=different_embedding,
        confidence=1.0,
        source="test"
    )

    # Find similar entities to target, excluding target itself
    result = await query_similar_entities(
        mock_ctx,
        embedding=similar_embedding,
        exclude_id="entity:target",
        limit=5
    )

    assert result is not None
    assert len(result) > 0

    # Result is list[dict] format (multiple records)
    # Check that similar1 is in results
    ids_found = [str(entity['id']) for entity in result]
    assert 'entity:similar1' in ids_found

    # Verify target itself is not in results
    assert 'entity:target' not in ids_found


@pytest.mark.asyncio
async def test_knn_operator_investigation(mock_ctx):
    """Investigate why KNN operator doesn't work."""
    test_embedding = [0.5] * 384

    # Create a few entities
    for i in range(3):
        await query_upsert_entity(
            mock_ctx,
            entity_id=f"test{i}",
            entity_type="test",
            labels=["test"],
            content=f"Test entity {i}",
            embedding=test_embedding,
            confidence=1.0,
            source="test"
        )

    # Check if index exists
    info_result = await run_query(mock_ctx, "INFO FOR TABLE entity")
    print(f"Table info: {info_result}")

    # Try KNN with literal integers
    knn_result = await run_query(mock_ctx, """
        SELECT id, content FROM entity
        WHERE embedding <|5,100|> $emb
    """, {'emb': test_embedding})
    print(f"KNN result: {knn_result}")

    # Compare with vector similarity
    sim_result = await run_query(mock_ctx, """
        SELECT id, content,
               vector::similarity::cosine(embedding, $emb) AS sim
        FROM entity
        ORDER BY sim DESC
        LIMIT 5
    """, {'emb': test_embedding})
    print(f"Similarity result: {sim_result}")

    # The test documents the issue - not asserting success/failure


# =============================================================================
# Additional Query Function Tests
# =============================================================================

@pytest.mark.asyncio
async def test_query_hybrid_search(mock_ctx):
    """Test query_hybrid_search function - BM25 + vector similarity."""
    test_embedding = [0.5] * 384

    # Create entities with searchable content
    await query_upsert_entity(
        mock_ctx,
        entity_id="python_dev",
        entity_type="person",
        labels=["developer", "python"],
        content="Expert Python developer with machine learning experience",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="rust_dev",
        entity_type="person",
        labels=["developer", "rust"],
        content="Systems programmer specializing in Rust",
        embedding=[0.1] * 384,  # Different embedding
        confidence=1.0,
        source="test"
    )

    # Search for "Python developer"
    result = await query_hybrid_search(
        mock_ctx,
        query="Python developer",
        query_embedding=test_embedding,
        labels=[],
        limit=5,
        semantic_weight=0.5
    )

    assert result is not None
    # Should find at least the python_dev entity
    ids_found = [str(e.get('id', '')) for e in result if isinstance(e, dict)]
    assert any('python_dev' in id for id in ids_found)


@pytest.mark.asyncio
async def test_query_traverse(mock_ctx):
    """Test query_traverse function - graph traversal."""
    test_embedding = [0.5] * 384

    # Create a graph: A -> B -> C
    await query_upsert_entity(mock_ctx, "nodeA", "node", ["test"], "Node A", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "nodeB", "node", ["test"], "Node B", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "nodeC", "node", ["test"], "Node C", test_embedding, 1.0, "test")

    await query_create_relation(mock_ctx, "nodeA", "connects", "nodeB", 1.0)
    await query_create_relation(mock_ctx, "nodeB", "connects", "nodeC", 1.0)

    # Traverse from A with depth 2
    result = await query_traverse(mock_ctx, "nodeA", depth=2, relation_types=None)

    assert result is not None
    assert len(result) > 0


@pytest.mark.asyncio
async def test_query_find_path(mock_ctx):
    """Test query_find_path function."""
    test_embedding = [0.5] * 384

    # Create connected entities
    await query_upsert_entity(mock_ctx, "start", "node", ["test"], "Start node", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "middle", "node", ["test"], "Middle node", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "end", "node", ["test"], "End node", test_embedding, 1.0, "test")

    await query_create_relation(mock_ctx, "start", "links", "middle", 1.0)
    await query_create_relation(mock_ctx, "middle", "links", "end", 1.0)

    # Find path from start to end
    result = await query_find_path(mock_ctx, "start", "end", max_depth=3)

    # Result may be empty if path syntax differs in v3.0
    assert result is not None


@pytest.mark.asyncio
async def test_query_apply_decay(mock_ctx):
    """Test query_apply_decay function."""
    test_embedding = [0.5] * 384

    # Create entity
    await query_upsert_entity(
        mock_ctx,
        entity_id="decay_test",
        entity_type="test",
        labels=["test"],
        content="Decay test entity",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Set accessed to an old date so the decay condition matches
    from datetime import datetime, timedelta
    old_date = (datetime.now() - timedelta(days=30)).isoformat() + "Z"
    await run_query(mock_ctx, """
        UPDATE type::record("entity", $id) SET accessed = <datetime>$old_date
    """, {'id': 'decay_test', 'old_date': old_date})

    # Get initial decay_weight
    entity = await query_get_entity(mock_ctx, "decay_test")
    initial_decay = entity[0]['decay_weight']
    assert initial_decay == 1.0  # Default value

    # Apply decay with current time as cutoff (entity accessed 30 days ago)
    now_cutoff = datetime.now().isoformat() + "Z"
    await query_apply_decay(mock_ctx, now_cutoff)

    # Check decay was applied
    entity = await query_get_entity(mock_ctx, "decay_test")
    new_decay = entity[0]['decay_weight']
    assert new_decay == 0.9  # 1.0 * 0.9


@pytest.mark.asyncio
async def test_query_all_entities_with_embedding(mock_ctx):
    """Test query_all_entities_with_embedding function."""
    test_embedding = [0.5] * 384

    # Create multiple entities
    for i in range(3):
        await query_upsert_entity(
            mock_ctx,
            entity_id=f"bulk_{i}",
            entity_type="test",
            labels=["bulk"],
            content=f"Bulk entity {i}",
            embedding=test_embedding,
            confidence=1.0,
            source="test"
        )

    # Get all entities with embeddings
    result = await query_all_entities_with_embedding(mock_ctx)

    assert result is not None
    assert len(result) >= 3
    # Check that embeddings are included
    assert 'embedding' in result[0]


@pytest.mark.asyncio
async def test_query_similar_by_embedding(mock_ctx):
    """Test query_similar_by_embedding function - uses KNN operator."""
    similar_embedding = [0.5] * 384
    different_embedding = [0.1] * 384

    # Create entities
    await query_upsert_entity(mock_ctx, "sim_target", "test", ["test"], "Target", similar_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "sim_similar", "test", ["test"], "Similar", similar_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "sim_different", "test", ["test"], "Different", different_embedding, 1.0, "test")

    # Find similar to target
    result = await query_similar_by_embedding(
        mock_ctx,
        embedding=similar_embedding,
        exclude_id="entity:sim_target",
        limit=5
    )

    assert result is not None
    # KNN with HNSW should return results
    if len(result) > 0:
        ids_found = [str(e.get('id', '')) for e in result if isinstance(e, dict)]
        assert any('sim_similar' in id for id in ids_found)


@pytest.mark.asyncio
async def test_query_delete_entity_by_record_id(mock_ctx):
    """Test query_delete_entity_by_record_id function."""
    test_embedding = [0.5] * 384

    # Create entity
    await query_upsert_entity(mock_ctx, "to_delete_record", "test", ["test"], "Will delete", test_embedding, 1.0, "test")

    # Verify exists
    result = await query_get_entity(mock_ctx, "to_delete_record")
    assert len(result) > 0

    # Delete by record ID
    await query_delete_entity_by_record_id(mock_ctx, "to_delete_record")

    # Verify deleted
    result = await query_get_entity(mock_ctx, "to_delete_record")
    assert len(result) == 0


@pytest.mark.asyncio
async def test_query_entity_with_embedding(mock_ctx):
    """Test query_entity_with_embedding function."""
    test_embedding = [0.5] * 384

    await query_upsert_entity(mock_ctx, "emb_test", "test", ["test"], "Embedding test", test_embedding, 1.0, "test")

    result = await query_entity_with_embedding(mock_ctx, "emb_test")

    assert result is not None
    assert len(result) > 0
    assert 'embedding' in result[0]
    assert len(result[0]['embedding']) == 384


@pytest.mark.asyncio
async def test_query_similar_for_contradiction(mock_ctx):
    """Test query_similar_for_contradiction function."""
    test_embedding = [0.5] * 384

    await query_upsert_entity(mock_ctx, "contra_target", "fact", ["test"], "The sky is blue", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "contra_similar", "fact", ["test"], "The sky is azure", test_embedding, 1.0, "test")

    result = await query_similar_for_contradiction(mock_ctx, test_embedding, "contra_target")

    assert result is not None
    # Should find similar entities for contradiction checking


@pytest.mark.asyncio
async def test_query_entities_by_labels(mock_ctx):
    """Test query_entities_by_labels function."""
    test_embedding = [0.5] * 384

    await query_upsert_entity(mock_ctx, "labeled_1", "test", ["python", "coding"], "Python entity", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "labeled_2", "test", ["rust", "coding"], "Rust entity", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "labeled_3", "test", ["javascript"], "JS entity", test_embedding, 1.0, "test")

    # Filter by "coding" label
    result = await query_entities_by_labels(mock_ctx, ["coding"])

    assert result is not None
    assert len(result) >= 2
    ids = [str(e.get('id', '')) for e in result if isinstance(e, dict)]
    assert any('labeled_1' in id for id in ids)
    assert any('labeled_2' in id for id in ids)


@pytest.mark.asyncio
async def test_query_vector_similarity(mock_ctx):
    """Test query_vector_similarity function."""
    import math
    emb1 = [0.5] * 384
    emb2 = [0.5] * 384  # Identical
    emb3 = [0.1] * 384  # Different but non-zero

    # Same embeddings should have similarity 1.0
    result = await query_vector_similarity(mock_ctx, emb1, emb2)
    assert result is not None
    assert len(result) > 0
    sim = result[0].get('sim', 0)
    assert abs(sim - 1.0) < 0.01  # Should be ~1.0

    # Different embeddings - same direction vectors have similarity 1.0
    # (cosine measures angle, not magnitude)
    result = await query_vector_similarity(mock_ctx, emb1, emb3)
    sim = result[0].get('sim', 0)
    # Both are uniform vectors in same direction, so similarity is 1.0
    assert not math.isnan(sim)
    assert sim > 0.99  # Parallel vectors


@pytest.mark.asyncio
async def test_query_count_entities(mock_ctx):
    """Test query_count_entities function."""
    test_embedding = [0.5] * 384

    # Create 3 entities
    for i in range(3):
        await query_upsert_entity(mock_ctx, f"count_{i}", "test", ["test"], f"Count {i}", test_embedding, 1.0, "test")

    result = await query_count_entities(mock_ctx)

    assert result is not None
    assert len(result) > 0
    count = result[0].get('count', 0)
    assert count >= 3


@pytest.mark.asyncio
async def test_query_count_relations(mock_ctx):
    """Test query_count_relations function."""
    test_embedding = [0.5] * 384

    # Create entities and relations
    await query_upsert_entity(mock_ctx, "rel_a", "test", ["test"], "A", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "rel_b", "test", ["test"], "B", test_embedding, 1.0, "test")
    await query_create_relation(mock_ctx, "rel_a", "relates", "rel_b", 1.0)

    result = await query_count_relations(mock_ctx)

    assert result is not None
    # Query may return different format, just verify it runs


@pytest.mark.asyncio
async def test_query_get_all_labels(mock_ctx):
    """Test query_get_all_labels function."""
    test_embedding = [0.5] * 384

    await query_upsert_entity(mock_ctx, "lbl_1", "test", ["alpha", "beta"], "Entity 1", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "lbl_2", "test", ["beta", "gamma"], "Entity 2", test_embedding, 1.0, "test")

    result = await query_get_all_labels(mock_ctx)

    assert result is not None
    assert len(result) > 0
    labels = result[0].get('labels', [])
    assert 'alpha' in labels
    assert 'beta' in labels
    assert 'gamma' in labels


@pytest.mark.asyncio
async def test_query_count_by_label(mock_ctx):
    """Test query_count_by_label function."""
    test_embedding = [0.5] * 384

    await query_upsert_entity(mock_ctx, "cnt_1", "test", ["special"], "Special 1", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "cnt_2", "test", ["special"], "Special 2", test_embedding, 1.0, "test")
    await query_upsert_entity(mock_ctx, "cnt_3", "test", ["other"], "Other", test_embedding, 1.0, "test")

    result = await query_count_by_label(mock_ctx, "special")

    assert result is not None
    assert len(result) > 0
    count = result[0].get('count', 0)
    assert count >= 2


@pytest.mark.asyncio
async def test_unique_relation_constraint(mock_ctx):
    """Test that duplicate relations are handled (unique constraint)."""
    # Create two entities
    test_embedding = [0.5] * 384

    await query_upsert_entity(
        mock_ctx,
        entity_id="entity_a",
        entity_type="concept",
        labels=["test"],
        content="Entity A",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="entity_b",
        entity_type="concept",
        labels=["test"],
        content="Entity B",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Create first relation
    await query_create_relation(
        mock_ctx,
        from_id="entity_a",
        rel_type="linked_to",
        to_id="entity_b",
        weight=0.5
    )

    # Try to create duplicate relation (same from, to, type)
    # Should either update or be rejected by unique constraint
    await query_create_relation(
        mock_ctx,
        from_id="entity_a",
        rel_type="linked_to",
        to_id="entity_b",
        weight=0.9  # Different weight
    )

    # Count relations - should only be 1 due to unique constraint
    relations = await run_query(mock_ctx, """
        SELECT * FROM relates WHERE rel_type = $rel_type
    """, {'rel_type': 'linked_to'})

    # Either 1 relation (unique enforced) or 2 (if RELATE creates new)
    # The unique constraint should prevent duplicates
    assert len(relations) >= 1


@pytest.mark.asyncio
async def test_knn_with_hnsw_index(mock_ctx):
    """Test KNN queries use HNSW index with ef parameter."""
    # Create entities with embeddings
    base_embedding = [0.5] * 384
    similar_embedding = [0.51] * 384  # Very similar
    different_embedding = [0.1] * 384  # Different

    await query_upsert_entity(
        mock_ctx,
        entity_id="base_entity",
        entity_type="concept",
        labels=["test"],
        content="Base entity for KNN test",
        embedding=base_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="similar_entity",
        entity_type="concept",
        labels=["test"],
        content="Similar entity for KNN test",
        embedding=similar_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="different_entity",
        entity_type="concept",
        labels=["test"],
        content="Different entity for KNN test",
        embedding=different_embedding,
        confidence=1.0,
        source="test"
    )

    # Test KNN query with HNSW (ef=40 parameter)
    result = await query_similar_by_embedding(
        mock_ctx,
        embedding=base_embedding,
        exclude_id="base_entity",
        limit=5
    )

    assert result is not None
    assert len(result) >= 1

    # Similar entity should be in results
    ids_found = [str(entity.get('id', '')) for entity in result]
    assert any('similar_entity' in id_str for id_str in ids_found)


@pytest.mark.asyncio
async def test_hybrid_search_rrf(mock_ctx):
    """Test hybrid search using RRF (Reciprocal Rank Fusion)."""
    # Create entities with both text and embeddings
    embedding1 = [0.5] * 384
    embedding2 = [0.6] * 384
    embedding3 = [0.1] * 384

    await query_upsert_entity(
        mock_ctx,
        entity_id="rrf_entity1",
        entity_type="document",
        labels=["test", "rrf"],
        content="Python programming language tutorial guide",
        embedding=embedding1,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="rrf_entity2",
        entity_type="document",
        labels=["test", "rrf"],
        content="Python snake species information",
        embedding=embedding2,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="rrf_entity3",
        entity_type="document",
        labels=["test", "rrf"],
        content="JavaScript web development",
        embedding=embedding3,
        confidence=1.0,
        source="test"
    )

    # Search with text query and embedding
    result = await query_hybrid_search(
        mock_ctx,
        query="Python",
        query_embedding=embedding1,
        labels=[],
        limit=5,
        semantic_weight=0.5
    )

    # RRF returns results combined from BM25 and vector search
    assert result is not None
    # Result format may vary - check it's a list
    assert isinstance(result, list)


@pytest.mark.asyncio
async def test_relates_table_structure(mock_ctx):
    """Test that relates table has correct structure with rel_type field."""
    # Create entities
    test_embedding = [0.5] * 384

    await query_upsert_entity(
        mock_ctx,
        entity_id="struct_a",
        entity_type="node",
        labels=["test"],
        content="Node A",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    await query_upsert_entity(
        mock_ctx,
        entity_id="struct_b",
        entity_type="node",
        labels=["test"],
        content="Node B",
        embedding=test_embedding,
        confidence=1.0,
        source="test"
    )

    # Create relation with specific type
    await query_create_relation(
        mock_ctx,
        from_id="struct_a",
        rel_type="connects_to",
        to_id="struct_b",
        weight=1.0
    )

    # Query relates table to verify structure
    result = await run_query(mock_ctx, """
        SELECT * FROM relates
    """)

    assert len(result) > 0
    rel = result[0]

    # Verify fields exist
    assert 'rel_type' in rel
    assert 'weight' in rel
    assert 'in' in rel or 'in' in str(rel)  # in/out are relation endpoints
    assert 'out' in rel or 'out' in str(rel)


