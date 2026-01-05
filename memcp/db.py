"""SurrealDB database connection and query management for memcp."""

import asyncio
import logging
import os
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Any, cast

from mcp.server.fastmcp import Context, FastMCP
from mcp.server.fastmcp.exceptions import ToolError
from surrealdb import AsyncSurreal

logger = logging.getLogger("memcp.db")

# Type alias for query results - SurrealDB returns list[Value] but we know
# our queries return list of dicts in practice
QueryResult = list[dict[str, Any]]

# Configuration from environment
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")
SURREALDB_NAMESPACE = os.getenv("SURREALDB_NAMESPACE", "knowledge")
SURREALDB_DATABASE = os.getenv("SURREALDB_DATABASE", "graph")
SURREALDB_USER = os.getenv("SURREALDB_USER", "root")
SURREALDB_PASS = os.getenv("SURREALDB_PASS", "root")
SURREALDB_AUTH_LEVEL = os.getenv("SURREALDB_AUTH_LEVEL", "root")  # "root" or "database"
QUERY_TIMEOUT = float(os.getenv("MEMCP_QUERY_TIMEOUT", "30"))

# Schema initialization SQL
SCHEMA_SQL = """
    DEFINE TABLE IF NOT EXISTS entity SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS type ON entity TYPE string;
    DEFINE FIELD IF NOT EXISTS labels ON entity TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS content ON entity TYPE string;
    DEFINE FIELD IF NOT EXISTS embedding ON entity TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS confidence ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS source ON entity TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS decay_weight ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON entity TYPE int DEFAULT 0;

    DEFINE INDEX IF NOT EXISTS entity_labels ON entity FIELDS labels;
    DEFINE INDEX IF NOT EXISTS entity_embedding ON entity FIELDS embedding HNSW DIMENSION 384 DIST COSINE;
    DEFINE ANALYZER IF NOT EXISTS entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;
"""


@dataclass
class AppContext:
    """Application context available during server lifetime."""
    db: AsyncSurreal
    initialized: bool = False


@asynccontextmanager
async def app_lifespan(server: FastMCP) -> AsyncIterator[AppContext]:
    """Manage database connection lifecycle."""
    logger.info(f"app_lifespan starting - URL: {SURREALDB_URL}, NS: {SURREALDB_NAMESPACE}, DB: {SURREALDB_DATABASE}")
    db = AsyncSurreal(SURREALDB_URL)
    ctx = AppContext(db=db)

    try:
        # Connect to database
        logger.info(f"Connecting to SurrealDB at {SURREALDB_URL}...")
        print(f"Connecting to SurrealDB at {SURREALDB_URL}...", file=sys.stderr)
        async with asyncio.timeout(QUERY_TIMEOUT):
            await db.connect()
            logger.info("Connected to SurrealDB")
            print("Connected to SurrealDB", file=sys.stderr)

            # Validate configuration
            if SURREALDB_AUTH_LEVEL not in ("root", "database"):
                raise ValueError(
                    f"Invalid SURREALDB_AUTH_LEVEL: '{SURREALDB_AUTH_LEVEL}'. "
                    f"Must be 'root' or 'database'"
                )

            print(f"Authenticating as {SURREALDB_USER} (auth level: {SURREALDB_AUTH_LEVEL})...", file=sys.stderr)

            if SURREALDB_AUTH_LEVEL == "root":
                # Root-level authentication
                await db.signin({
                    "username": SURREALDB_USER,
                    "password": SURREALDB_PASS
                })
            else:
                # Database-level authentication
                await db.signin({
                    "namespace": SURREALDB_NAMESPACE,
                    "database": SURREALDB_DATABASE,
                    "username": SURREALDB_USER,
                    "password": SURREALDB_PASS
                })

            print("Authentication successful", file=sys.stderr)

            print(f"Using namespace '{SURREALDB_NAMESPACE}' and database '{SURREALDB_DATABASE}'", file=sys.stderr)
            await db.use(SURREALDB_NAMESPACE, SURREALDB_DATABASE)

        # Initialize schema
        print("Initializing database schema...", file=sys.stderr)
        await db.query(cast(Any, SCHEMA_SQL))
        print("Schema initialization complete", file=sys.stderr)

        ctx.initialized = True
        print("MCP server ready", file=sys.stderr)
        yield ctx

    except asyncio.TimeoutError:
        print(f"ERROR: Database operation timed out after {QUERY_TIMEOUT}s", file=sys.stderr)
        print(f"Check that SurrealDB is running at {SURREALDB_URL}", file=sys.stderr)
        raise
    except Exception as e:
        print(f"ERROR: Failed to initialize MCP server: {e}", file=sys.stderr)
        if "IAM error" in str(e) or "permissions" in str(e).lower():
            print("\nAuthentication failed!", file=sys.stderr)
            print(f"Current auth level: {SURREALDB_AUTH_LEVEL}", file=sys.stderr)

            if SURREALDB_AUTH_LEVEL == "root":
                print("\nFor root-level authentication:", file=sys.stderr)
                print("  export SURREALDB_USER=root", file=sys.stderr)
                print("  export SURREALDB_PASS=root", file=sys.stderr)
                print("\nStart SurrealDB with:", file=sys.stderr)
                print("  docker run -p 8000:8000 surrealdb/surrealdb:latest \\", file=sys.stderr)
                print("    start --user root --pass root file:/data/database.db", file=sys.stderr)
            else:
                print("\nFor database-level authentication:", file=sys.stderr)
                print("  export SURREALDB_NAMESPACE=knowledge", file=sys.stderr)
                print("  export SURREALDB_DATABASE=graph", file=sys.stderr)
                print("  export SURREALDB_USER=your_user", file=sys.stderr)
                print("  export SURREALDB_PASS=your_pass", file=sys.stderr)
                print("\nOr switch to root-level:", file=sys.stderr)
                print("  export SURREALDB_AUTH_LEVEL=root", file=sys.stderr)
        raise
    finally:
        # Cleanup on shutdown
        if ctx.initialized:
            try:
                await db.close()
            except Exception:
                pass


def get_db(ctx: Context) -> AsyncSurreal:
    """Get database connection from context."""
    app_ctx: AppContext = ctx.request_context.lifespan_context
    return app_ctx.db


async def run_query(ctx: Context, sql: str, vars: dict[str, Any] | None = None) -> QueryResult:
    """Execute a database query with timeout and error handling."""
    db = get_db(ctx)
    # Log query (truncate vars to avoid huge embeddings in logs)
    vars_summary = {k: f"[{len(v)} items]" if isinstance(v, list) and len(v) > 10 else v for k, v in (vars or {}).items()}
    logger.debug(f"run_query: {sql[:100]}... vars={vars_summary}")
    try:
        async with asyncio.timeout(QUERY_TIMEOUT):
            result = await db.query(sql, cast(Any, vars))
            logger.debug(f"Query result type: {type(result)}, len: {len(result) if isinstance(result, list) else 'N/A'}")
            return cast(QueryResult, result)
    except asyncio.TimeoutError:
        logger.error(f"Query timed out after {QUERY_TIMEOUT}s")
        await ctx.error(f"Query timed out after {QUERY_TIMEOUT}s")
        raise ToolError(f"Database query timed out after {QUERY_TIMEOUT}s")
    except ToolError:
        raise
    except Exception as e:
        logger.error(f"Query failed: {e}", exc_info=True)
        await ctx.error(f"Database query failed: {e}")
        raise ToolError(f"Database query failed: {e}")


def validate_entity(entity: dict) -> None:
    """Validate entity structure before storing."""
    if not isinstance(entity, dict):
        raise ToolError(f"Entity must be a dict, got {type(entity).__name__}")
    if "id" not in entity:
        raise ToolError("Entity missing required field: 'id'")
    if "content" not in entity:
        raise ToolError("Entity missing required field: 'content'")
    if not isinstance(entity["id"], str) or not entity["id"].strip():
        raise ToolError("Entity 'id' must be a non-empty string")
    if not isinstance(entity["content"], str) or not entity["content"].strip():
        raise ToolError("Entity 'content' must be a non-empty string")
    if "labels" in entity and not isinstance(entity.get("labels"), list):
        raise ToolError("Entity 'labels' must be a list")
    if "confidence" in entity:
        conf = entity["confidence"]
        if not isinstance(conf, (int, float)) or not 0 <= conf <= 1:
            raise ToolError("Entity 'confidence' must be a number between 0 and 1")


def validate_relation(relation: dict) -> None:
    """Validate relation structure before storing."""
    if not isinstance(relation, dict):
        raise ToolError(f"Relation must be a dict, got {type(relation).__name__}")
    for field in ("from", "to", "type"):
        if field not in relation:
            raise ToolError(f"Relation missing required field: '{field}'")
        if not isinstance(relation[field], str) or not relation[field].strip():
            raise ToolError(f"Relation '{field}' must be a non-empty string")
    if "weight" in relation:
        weight = relation["weight"]
        if not isinstance(weight, (int, float)):
            raise ToolError("Relation 'weight' must be a number")


# =============================================================================
# Query Functions - Extracted from MCP tools for testability
# =============================================================================

async def query_hybrid_search(
    ctx: Context,
    query: str,
    query_embedding: list[float],
    labels: list[str],
    limit: int,
    semantic_weight: float
) -> QueryResult:
    """Hybrid search: BM25 keyword matching + vector similarity."""
    label_filter = "AND labels CONTAINSANY $labels" if labels else ""
    # v3.0: ORDER BY doesn't support parentheses, compute combined score as alias
    return await run_query(ctx, f"""
        SELECT id, type, labels, content, confidence, source, decay_weight,
               search::score(1) AS bm25_score,
               vector::similarity::cosine(embedding, $emb) AS vec_score,
               vector::similarity::cosine(embedding, $emb) * $sem_weight + search::score(1) * 1 - $sem_weight AS combined_score
        FROM entity
        WHERE content @1@ $q OR embedding <|{limit * 2},COSINE|> $emb {label_filter}
        ORDER BY combined_score DESC
        LIMIT $limit
    """, {
        'q': query,
        'emb': query_embedding,
        'labels': labels,
        'limit': limit,
        'sem_weight': semantic_weight
    })


async def query_update_access(ctx: Context, entity_id: str) -> QueryResult:
    """Update entity access timestamp and count."""
    return await run_query(ctx, """
        UPDATE type::record("entity", $id) SET accessed = time::now(), access_count += 1
    """, {'id': entity_id})


async def query_get_entity(ctx: Context, entity_id: str) -> QueryResult:
    """Get entity by ID.

    Returns: list[dict] where dict is the single entity, or empty list if not found.
    """
    return await run_query(ctx, """
        SELECT * FROM type::record("entity", $id)
    """, {'id': entity_id})


async def query_list_labels(ctx: Context) -> QueryResult:
    """List all unique labels from entities."""
    # SELECT FROM entity returns list[dict] - each dict has labels field
    result = await run_query(ctx, "SELECT labels FROM entity")

    if not result or len(result) == 0:
        return [{'labels': []}]

    all_labels = []

    # Result is list[dict] where each dict is {'labels': [...]}
    for entity in result:
        if isinstance(entity, dict) and 'labels' in entity and entity['labels']:
            all_labels.extend(entity['labels'])

    # Deduplicate
    unique_labels = list(set(all_labels))

    # Return in expected format
    return [{'labels': unique_labels}]


async def query_traverse(ctx: Context, entity_id: str, depth: int, relation_types: list[str] | None) -> QueryResult:
    """Traverse graph from entity with optional relation type filter."""
    if relation_types:
        type_filter = '|'.join(relation_types)
        return await run_query(ctx, f"""
            SELECT *, ->(({type_filter}))..{depth}->entity AS connected
            FROM type::record("entity", $id)
        """, {'id': entity_id})
    else:
        return await run_query(ctx, f"""
            SELECT *, ->?..{depth}->entity AS connected
            FROM type::record("entity", $id)
        """, {'id': entity_id})


async def query_find_path(ctx: Context, from_id: str, to_id: str, max_depth: int) -> QueryResult:
    """Find path between two entities."""
    return await run_query(ctx, f"""
        SELECT * FROM type::record("entity", $from)..{max_depth}->entity WHERE id = type::record("entity", $to) LIMIT 1
    """, {'from': from_id, 'to': to_id})


async def query_similar_entities(ctx: Context, embedding: list[float], exclude_id: str, limit: int = 5) -> QueryResult:
    """Find similar entities by embedding.

    Note: Uses vector::similarity::cosine instead of KNN operator <|...|>
    because MTREE index may not be available in all SurrealDB environments.

    Args:
        exclude_id: Can be either "entity:id" (full record ID) or just "id" (entity ID only)
    """
    # Extract just the entity ID part if full record ID is provided
    entity_only_id = exclude_id.split(':', 1)[1] if ':' in exclude_id else exclude_id

    return await run_query(ctx, f"""
        SELECT id, content,
               vector::similarity::cosine(embedding, $emb) AS sim
        FROM entity
        WHERE id != type::record("entity", "{entity_only_id}")
        ORDER BY sim DESC
        LIMIT $limit
    """, {
        'emb': embedding,
        'limit': limit
    })


async def query_upsert_entity(
    ctx: Context,
    entity_id: str,
    entity_type: str,
    labels: list[str],
    content: str,
    embedding: list[float],
    confidence: float,
    source: str | None
) -> QueryResult:
    """Upsert an entity."""
    return await run_query(ctx, """
        UPSERT type::record("entity", $id) SET
            type = $type,
            labels = $labels,
            content = $content,
            embedding = $embedding,
            confidence = $confidence,
            source = $source
    """, {
        'id': entity_id,
        'type': entity_type,
        'labels': labels,
        'content': content,
        'embedding': embedding,
        'confidence': confidence,
        'source': source
    })


async def query_create_relation(
    ctx: Context,
    from_id: str,
    rel_type: str,
    to_id: str,
    weight: float
) -> QueryResult:
    """Create a relation between entities."""
    # Build record IDs and use string interpolation for relation type
    return await run_query(ctx, f"""
        RELATE entity:{from_id}->{rel_type}->entity:{to_id} SET weight = $weight
    """, {
        'weight': weight
    })


async def query_delete_entity(ctx: Context, entity_id: str) -> QueryResult:
    """Delete entity and all its relations."""
    # Delete entity and its relations in separate statements
    await run_query(ctx, "DELETE type::record('entity', $id)", {'id': entity_id})
    # Note: Relations are automatically cleaned up by SurrealDB when the record is deleted
    return []  # Return empty result


async def query_apply_decay(ctx: Context, cutoff_datetime: str) -> QueryResult:
    """Apply temporal decay to old entities."""
    # v3.0: Must cast string parameter to datetime for comparison
    return await run_query(ctx, """
        UPDATE entity SET decay_weight = decay_weight * 0.9
        WHERE accessed < <datetime>$cutoff AND decay_weight > 0.1
    """, {'cutoff': cutoff_datetime})


async def query_all_entities_with_embedding(ctx: Context) -> QueryResult:
    """Get all entities with their embeddings for similarity comparison."""
    return await run_query(ctx, "SELECT id, content, embedding, access_count, accessed FROM entity")


async def query_similar_by_embedding(
    ctx: Context,
    embedding: list[float],
    exclude_id: str,
    limit: int = 10
) -> QueryResult:
    """Find similar entities by embedding with similarity score."""
    return await run_query(ctx, f"""
        SELECT id, content, access_count, accessed,
               vector::similarity::cosine(embedding, $emb) AS sim
        FROM entity
        WHERE embedding <|{limit},COSINE|> $emb AND id != $exclude_id
    """, {'emb': embedding, 'exclude_id': exclude_id})


async def query_delete_entity_by_record_id(ctx: Context, entity_id: str) -> QueryResult:
    """Delete entity by record ID (used in reflect merge).

    Note: SurrealDB automatically cleans up relations when the record is deleted.
    """
    return await run_query(ctx, """
        DELETE type::record("entity", $id)
    """, {'id': entity_id})


async def query_entity_with_embedding(ctx: Context, entity_id: str) -> QueryResult:
    """Get entity with embedding for contradiction checking."""
    return await run_query(ctx, 'SELECT * FROM type::record("entity", $id)', {'id': entity_id})


async def query_similar_for_contradiction(
    ctx: Context,
    embedding: list[float],
    entity_id: str
) -> QueryResult:
    """Find similar entities for contradiction detection."""
    return await run_query(ctx, """
        SELECT id, content FROM entity
        WHERE embedding <|10,COSINE|> $emb AND id != $id
    """, {'emb': embedding, 'id': f"entity:{entity_id}"})


async def query_entities_by_labels(ctx: Context, labels: list[str]) -> QueryResult:
    """Get entities filtered by labels."""
    label_filter = "WHERE labels CONTAINSANY $labels" if labels else ""
    return await run_query(ctx, f"SELECT id, content, embedding FROM entity {label_filter}", {'labels': labels})


async def query_vector_similarity(ctx: Context, emb1: list[float], emb2: list[float]) -> QueryResult:
    """Calculate cosine similarity between two embeddings."""
    # v3.0: SELECT requires FROM, use RETURN for computed values
    # Wrap in list for consistent QueryResult format
    result = await run_query(ctx, """
        RETURN { sim: vector::similarity::cosine($emb1, $emb2) }
    """, {'emb1': emb1, 'emb2': emb2})
    # RETURN gives dict directly, wrap in list
    if isinstance(result, dict):
        return [result]
    return result


async def query_count_entities(ctx: Context) -> QueryResult:
    """Count total entities."""
    return await run_query(ctx, "SELECT count() FROM entity GROUP ALL")


async def query_count_relations(ctx: Context) -> QueryResult:
    """Count total relations."""
    return await run_query(ctx, """
        SELECT count() FROM (SELECT ->? FROM entity) GROUP ALL
    """)


async def query_get_all_labels(ctx: Context) -> QueryResult:
    """Get all unique labels."""
    # v3.0: Subquery with .field syntax changed, use Python-side processing
    result = await run_query(ctx, "SELECT labels FROM entity")
    if not result:
        return [{'labels': []}]
    all_labels = []
    for entity in result:
        if isinstance(entity, dict) and 'labels' in entity and entity['labels']:
            all_labels.extend(entity['labels'])
    return [{'labels': list(set(all_labels))}]


async def query_count_by_label(ctx: Context, label: str) -> QueryResult:
    """Count entities with a specific label."""
    return await run_query(ctx, """
        SELECT count() FROM entity WHERE labels CONTAINS $label GROUP ALL
    """, {'label': label})


__all__ = [
    'AppContext',
    'app_lifespan',
    'get_db',
    'run_query',
    'validate_entity',
    'validate_relation',
    'QueryResult',
    'SURREALDB_URL',
    'SURREALDB_NAMESPACE',
    'SURREALDB_DATABASE',
    'SCHEMA_SQL',
    # Query functions
    'query_hybrid_search',
    'query_update_access',
    'query_get_entity',
    'query_list_labels',
    'query_traverse',
    'query_find_path',
    'query_similar_entities',
    'query_upsert_entity',
    'query_create_relation',
    'query_delete_entity',
    'query_apply_decay',
    'query_all_entities_with_embedding',
    'query_similar_by_embedding',
    'query_delete_entity_by_record_id',
    'query_entity_with_embedding',
    'query_similar_for_contradiction',
    'query_entities_by_labels',
    'query_vector_similarity',
    'query_count_entities',
    'query_count_relations',
    'query_get_all_labels',
    'query_count_by_label',
]
