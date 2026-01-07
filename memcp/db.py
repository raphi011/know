"""SurrealDB database connection and query management for memcp."""

import asyncio
import logging
import os
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Any, cast

from fastmcp import Context, FastMCP
from fastmcp.exceptions import ToolError
from surrealdb import AsyncSurreal, RecordID

# Type alias for database connection - allows both direct db and MCP context usage
DBConnection = AsyncSurreal

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

# Context detection configuration
MEMCP_DEFAULT_CONTEXT = os.getenv("MEMCP_DEFAULT_CONTEXT", None)
MEMCP_CONTEXT_FROM_CWD = os.getenv("MEMCP_CONTEXT_FROM_CWD", "false").lower() == "true"

# =============================================================================
# Entity Type Ontology
# =============================================================================
# Predefined entity types for structured knowledge extraction
# Based on Graphiti's built-in types + custom additions

ENTITY_TYPES = {
    # Core types
    "concept": "General knowledge or idea",
    "fact": "Verified piece of information",
    "preference": "User preference or choice",
    "requirement": "Requirement or constraint",
    "decision": "Decision that was made",

    # People & Organizations
    "person": "A person",
    "organization": "Company, team, or group",

    # Location & Time
    "location": "Physical or virtual location",
    "event": "Something that happened at a specific time",

    # Technical
    "tool": "Software tool or technology",
    "project": "A project or initiative",
    "code": "Code snippet or technical implementation",

    # Procedural (for procedural memory)
    "procedure": "Step-by-step workflow or process",
    "step": "Single step within a procedure",
}

# Allow custom types beyond the predefined ones
ALLOW_CUSTOM_TYPES = os.getenv("MEMCP_ALLOW_CUSTOM_TYPES", "true").lower() == "true"


def validate_entity_type(entity_type: str) -> str:
    """Validate and normalize entity type.

    Returns the normalized type if valid, raises ToolError if invalid.
    """
    normalized = entity_type.lower().strip()

    if normalized in ENTITY_TYPES:
        return normalized

    if ALLOW_CUSTOM_TYPES:
        return normalized

    valid_types = ", ".join(sorted(ENTITY_TYPES.keys()))
    raise ToolError(f"Invalid entity type '{entity_type}'. Valid types: {valid_types}")


def get_entity_types() -> dict[str, str]:
    """Get all predefined entity types with descriptions."""
    return ENTITY_TYPES.copy()


def _get_git_origin_name() -> str | None:
    """Extract project name from git remote origin URL.

    Handles various URL formats:
    - git@github.com:owner/repo.git -> owner/repo
    - https://github.com/owner/repo.git -> owner/repo
    - https://github.com/owner/repo -> owner/repo
    """
    import subprocess
    try:
        result = subprocess.run(
            ['git', 'config', '--get', 'remote.origin.url'],
            capture_output=True, text=True, timeout=5
        )
        if result.returncode != 0 or not result.stdout.strip():
            return None

        url = result.stdout.strip()

        # Handle SSH format: git@github.com:owner/repo.git
        if url.startswith('git@'):
            # git@github.com:owner/repo.git -> owner/repo.git
            path = url.split(':', 1)[-1]
        # Handle HTTPS format: https://github.com/owner/repo.git
        elif '://' in url:
            # https://github.com/owner/repo.git -> owner/repo.git
            path = url.split('://', 1)[-1].split('/', 1)[-1]
        else:
            return None

        # Remove .git suffix and return
        if path.endswith('.git'):
            path = path[:-4]

        return path if path else None
    except Exception:
        return None


def detect_context(explicit_context: str | None = None) -> str | None:
    """Detect project context from explicit value, env, git origin, or cwd.

    Priority:
    1. Explicit context parameter (if provided)
    2. MEMCP_DEFAULT_CONTEXT env var (if set)
    3. Git remote origin name (if MEMCP_CONTEXT_FROM_CWD=true and in a git repo)
    4. Current working directory basename (if MEMCP_CONTEXT_FROM_CWD=true)
    5. None (no context filtering)
    """
    if explicit_context:
        return explicit_context
    if MEMCP_DEFAULT_CONTEXT:
        return MEMCP_DEFAULT_CONTEXT
    if MEMCP_CONTEXT_FROM_CWD:
        # Try git origin first (handles worktrees correctly)
        git_origin = _get_git_origin_name()
        if git_origin:
            return git_origin
        # Fall back to cwd basename
        cwd = os.getcwd()
        return os.path.basename(cwd) if cwd else None
    return None


# Schema initialization SQL
SCHEMA_SQL = """
    -- ==========================================================================
    -- ENTITY TABLE
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS entity SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS type ON entity TYPE string;
    -- TODO: Use set<string> when Python SDK supports CBOR tag 56 (v3.0 set type)
    DEFINE FIELD IF NOT EXISTS labels ON entity TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS content ON entity TYPE string;
    DEFINE FIELD IF NOT EXISTS embedding ON entity TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS confidence ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS source ON entity TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS decay_weight ON entity TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON entity TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON entity TYPE int DEFAULT 0;
    -- Project namespacing: isolate memories by context
    DEFINE FIELD IF NOT EXISTS context ON entity TYPE option<string>;
    -- Importance scoring: heuristic-based salience
    DEFINE FIELD IF NOT EXISTS importance ON entity TYPE float DEFAULT 0.5;
    DEFINE FIELD IF NOT EXISTS user_importance ON entity TYPE option<float>;

    DEFINE INDEX IF NOT EXISTS entity_labels ON entity FIELDS labels;
    DEFINE INDEX IF NOT EXISTS entity_context ON entity FIELDS context;
    DEFINE INDEX IF NOT EXISTS entity_embedding ON entity FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;

    -- ==========================================================================
    -- RELATIONS TABLE
    -- ==========================================================================
    -- Relation table with unique constraint to prevent duplicate edges
    -- Uses single table with rel_type field instead of dynamic table names
    DEFINE TABLE IF NOT EXISTS relates TYPE RELATION IN entity OUT entity SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS rel_type ON relates TYPE string;
    DEFINE FIELD IF NOT EXISTS weight ON relates TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON relates TYPE datetime DEFAULT time::now();
    -- Unique constraint: sorted [in, out, rel_type] prevents duplicate relations
    DEFINE FIELD IF NOT EXISTS unique_key ON relates VALUE <string>string::concat(array::sort([<string>in, <string>out]), rel_type);
    DEFINE INDEX IF NOT EXISTS unique_relation ON relates FIELDS unique_key UNIQUE;

    -- ==========================================================================
    -- EPISODE TABLE (Episodic Memory)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS episode SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS content ON episode TYPE string;
    DEFINE FIELD IF NOT EXISTS summary ON episode TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS embedding ON episode TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS metadata ON episode TYPE option<object> FLEXIBLE;
    DEFINE FIELD IF NOT EXISTS timestamp ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS context ON episode TYPE option<string>;
    DEFINE FIELD IF NOT EXISTS created ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON episode TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON episode TYPE int DEFAULT 0;

    DEFINE INDEX IF NOT EXISTS episode_timestamp ON episode FIELDS timestamp;
    DEFINE INDEX IF NOT EXISTS episode_context ON episode FIELDS context;
    DEFINE INDEX IF NOT EXISTS episode_embedding ON episode FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS episode_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS episode_content_ft ON episode FIELDS content FULLTEXT ANALYZER episode_analyzer BM25;

    -- ==========================================================================
    -- EXTRACTED_FROM RELATION (links entities to source episodes)
    -- ==========================================================================
    DEFINE TABLE IF NOT EXISTS extracted_from TYPE RELATION IN entity OUT episode SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS position ON extracted_from TYPE option<int>;
    DEFINE FIELD IF NOT EXISTS confidence ON extracted_from TYPE float DEFAULT 1.0;
    DEFINE FIELD IF NOT EXISTS created ON extracted_from TYPE datetime DEFAULT time::now();

    -- ==========================================================================
    -- PROCEDURE TABLE (Procedural Memory)
    -- ==========================================================================
    -- Stores step-by-step workflows/processes with ordered steps
    DEFINE TABLE IF NOT EXISTS procedure SCHEMAFULL;
    DEFINE FIELD IF NOT EXISTS name ON procedure TYPE string;
    DEFINE FIELD IF NOT EXISTS description ON procedure TYPE string;
    DEFINE FIELD IF NOT EXISTS steps ON procedure TYPE array<object> FLEXIBLE;  -- [{order, content, optional}]
    -- Note: Must REMOVE then DEFINE to ensure FLEXIBLE is set (IF NOT EXISTS won't update existing field)
    REMOVE FIELD IF EXISTS steps.* ON procedure;
    DEFINE FIELD steps.* ON procedure TYPE object FLEXIBLE;  -- Allow nested object properties
    DEFINE FIELD IF NOT EXISTS embedding ON procedure TYPE array<float>;
    DEFINE FIELD IF NOT EXISTS context ON procedure TYPE option<string>;
    -- TODO: Use set<string> when Python SDK supports CBOR tag 56 (v3.0 set type)
    DEFINE FIELD IF NOT EXISTS labels ON procedure TYPE array<string>;
    DEFINE FIELD IF NOT EXISTS created ON procedure TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS accessed ON procedure TYPE datetime DEFAULT time::now();
    DEFINE FIELD IF NOT EXISTS access_count ON procedure TYPE int DEFAULT 0;

    DEFINE INDEX IF NOT EXISTS procedure_context ON procedure FIELDS context;
    DEFINE INDEX IF NOT EXISTS procedure_labels ON procedure FIELDS labels;
    DEFINE INDEX IF NOT EXISTS procedure_embedding ON procedure FIELDS embedding HNSW DIMENSION 384 DIST COSINE TYPE F32;
    DEFINE ANALYZER IF NOT EXISTS procedure_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
    DEFINE INDEX IF NOT EXISTS procedure_name_ft ON procedure FIELDS name FULLTEXT ANALYZER procedure_analyzer BM25;
    DEFINE INDEX IF NOT EXISTS procedure_desc_ft ON procedure FIELDS description FULLTEXT ANALYZER procedure_analyzer BM25;

    -- ==========================================================================
    -- TYPE INDEX (for entity type ontology queries)
    -- ==========================================================================
    DEFINE INDEX IF NOT EXISTS entity_type ON entity FIELDS type;
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


async def run_query(db: AsyncSurreal, sql: str, vars: dict[str, Any] | None = None) -> QueryResult:
    """Execute a database query with error handling.

    Args:
        db: AsyncSurreal database connection
        sql: SQL query string
        vars: Optional query variables

    Returns:
        Query results as list of dicts
    """
    # Log query (truncate vars to avoid huge embeddings in logs)
    vars_summary = {k: f"[{len(v)} items]" if isinstance(v, list) and len(v) > 10 else v for k, v in (vars or {}).items()}
    logger.debug(f"run_query: {sql[:100]}... vars={vars_summary}")
    try:
        result = await db.query(sql, cast(Any, vars))
        logger.debug(f"Query result type: {type(result)}, len: {len(result) if isinstance(result, list) else 'N/A'}")
        return cast(QueryResult, result)
    except ToolError:
        raise
    except asyncio.CancelledError:
        # SDK bug: CancelledError usually means CBOR decode failed (e.g., unsupported tag 56 for sets)
        # See: https://github.com/surrealdb/surrealdb.py/issues (CBOR tag 56 not supported)
        logger.error("Query cancelled - likely SDK CBOR decode error (unsupported type in response)")
        raise ToolError(
            "Database query failed: SDK connection error. "
            "This may be caused by SurrealDB returning a 'set' type which the Python SDK doesn't support. "
            "Ensure schema uses array<T> instead of set<T>."
        )
    except KeyError as e:
        # SDK bug: KeyError on query ID means _recv_task crashed during decode
        logger.error(f"Query failed with KeyError: {e} - likely SDK CBOR decode error")
        raise ToolError(
            f"Database query failed: SDK internal error ({e}). "
            "This may be caused by SurrealDB returning a 'set' type which the Python SDK doesn't support."
        )
    except Exception as e:
        logger.error(f"Query failed: {e}", exc_info=True)
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


def _extract_id(record_id: str | Any) -> str:
    """Extract entity ID from full record ID (internal helper)."""
    record_str = str(record_id)
    return record_str.split(':', 1)[1] if ':' in record_str else record_str


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
    db: AsyncSurreal,
    query: str,
    query_embedding: list[float],
    labels: list[str],
    limit: int,
    context: str | None = None
) -> QueryResult:
    """Hybrid search using Reciprocal Rank Fusion (RRF) to combine BM25 + vector results.

    RRF combines rankings without needing to normalize incompatible score scales.
    Formula: score = 1/(rank + k) where k=60 is a smoothing constant.
    """
    label_filter = "AND labels CONTAINSANY $labels" if labels else ""
    context_filter = "AND context = $context" if context else ""

    # Use search::rrf() to combine BM25 and vector search results
    # Using inline subqueries (LET variables don't work with search::rrf)
    return await run_query(db, f"""
        SELECT * FROM search::rrf([
            (SELECT id, type, labels, content, confidence, source, decay_weight, context, importance, accessed, access_count
             FROM entity
             WHERE embedding <|{limit * 2},40|> $emb {label_filter} {context_filter}),
            (SELECT id, type, labels, content, confidence, source, decay_weight, context, importance, accessed, access_count
             FROM entity
             WHERE content @0@ $q {label_filter} {context_filter})
        ], $limit, 60)
    """, {
        'q': query,
        'emb': query_embedding,
        'labels': labels,
        'context': context,
        'limit': limit
    })


async def query_update_access(db: AsyncSurreal, entity_id: str) -> QueryResult:
    """Update entity access timestamp, count, and reset decay weight."""
    return await run_query(db, """
        UPDATE type::record("entity", $id) SET accessed = time::now(), access_count += 1, decay_weight = 1.0
    """, {'id': entity_id})


async def query_get_entity(db: AsyncSurreal, entity_id: str) -> QueryResult:
    """Get entity by ID.

    Returns: list[dict] where dict is the single entity, or empty list if not found.
    """
    return await run_query(db, """
        SELECT * FROM type::record("entity", $id)
    """, {'id': entity_id})


async def query_list_labels(db: AsyncSurreal) -> QueryResult:
    """List all unique labels from entities."""
    # SELECT FROM entity returns list[dict] - each dict has labels field
    result = await run_query(db, "SELECT labels FROM entity")

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


async def query_traverse(db: AsyncSurreal, entity_id: str, depth: int, relation_types: list[str] | None) -> QueryResult:
    """Traverse graph from entity with optional relation type filter.

    Uses single 'relates' table with rel_type field for filtering.
    """
    if relation_types:
        # Filter by rel_type field within the relates table
        return await run_query(db, f"""
            SELECT *, ->(SELECT * FROM relates WHERE rel_type IN $types)..{depth}->entity AS connected
            FROM type::record("entity", $id)
        """, {'id': entity_id, 'types': relation_types})
    else:
        return await run_query(db, f"""
            SELECT *, ->relates..{depth}->entity AS connected
            FROM type::record("entity", $id)
        """, {'id': entity_id})


async def query_find_path(db: AsyncSurreal, from_id: str, to_id: str, max_depth: int) -> QueryResult:
    """Find path between two entities via relates table."""
    return await run_query(db, f"""
        SELECT * FROM type::record("entity", $from)->relates..{max_depth}->entity WHERE id = type::record("entity", $to) LIMIT 1
    """, {'from': from_id, 'to': to_id})


async def query_similar_entities(db: AsyncSurreal, embedding: list[float], exclude_id: str, limit: int = 5) -> QueryResult:
    """Find similar entities by embedding.

    Note: Uses vector::similarity::cosine instead of KNN operator <|...|>
    because MTREE index may not be available in all SurrealDB environments.

    Args:
        exclude_id: Can be either "entity:id" (full record ID) or just "id" (entity ID only)
    """
    return await run_query(db, f"""
        SELECT id, content,
               vector::similarity::cosine(embedding, $emb) AS sim
        FROM entity
        WHERE id != $exclude_rec
        ORDER BY sim DESC
        LIMIT $limit
    """, {
        'emb': embedding,
        'exclude_rec': RecordID('entity', _extract_id(exclude_id)),
        'limit': limit
    })


async def query_upsert_entity(
    db: AsyncSurreal,
    entity_id: str,
    entity_type: str,
    labels: list[str],
    content: str,
    embedding: list[float],
    confidence: float,
    source: str | None,
    context: str | None = None,
    user_importance: float | None = None
) -> QueryResult:
    """Upsert an entity with optional context and importance."""
    # Build SET clause dynamically based on provided values
    # TODO: Use <set<string>>$labels when Python SDK supports CBOR tag 56
    set_clause = """
            type = $type,
            labels = array::distinct($labels),
            content = $content,
            embedding = $embedding,
            confidence = $confidence,
            source = $source,
            context = $context
    """
    # Only set user_importance if explicitly provided (not None)
    if user_importance is not None:
        set_clause += ", user_importance = $user_importance"

    return await run_query(db, f"""
        UPSERT type::record("entity", $id) SET {set_clause}
    """, {
        'id': entity_id,
        'type': entity_type,
        'labels': labels,
        'content': content,
        'embedding': embedding,
        'confidence': confidence,
        'source': source,
        'context': context,
        'user_importance': user_importance
    })


async def query_create_relation(
    db: AsyncSurreal,
    from_id: str,
    rel_type: str,
    to_id: str,
    weight: float
) -> QueryResult:
    """Create a relation between entities.

    Uses single 'relates' table with rel_type field for unique constraint.
    Duplicate relations (same from, to, type) are silently ignored.
    """
    # Use RecordID for proper escaping of IDs with special characters (hyphens, etc.)
    # UPSERT-like behavior: if relation exists, update weight; otherwise create
    return await run_query(db, """
        RELATE $from_rec->relates->$to_rec SET rel_type = $rel_type, weight = $weight
    """, {
        'from_rec': RecordID('entity', from_id),
        'to_rec': RecordID('entity', to_id),
        'rel_type': rel_type,
        'weight': weight
    })


async def query_delete_entity(db: AsyncSurreal, entity_id: str) -> QueryResult:
    """Delete entity and all its relations."""
    # Delete entity and its relations in separate statements
    await run_query(db, "DELETE type::record('entity', $id)", {'id': entity_id})
    # Note: Relations are automatically cleaned up by SurrealDB when the record is deleted
    return []  # Return empty result


async def query_apply_decay(db: AsyncSurreal, cutoff_datetime: str) -> QueryResult:
    """Apply temporal decay to old entities."""
    # v3.0: Must cast string parameter to datetime for comparison
    return await run_query(db, """
        UPDATE entity SET decay_weight = decay_weight * 0.9
        WHERE accessed < <datetime>$cutoff AND decay_weight > 0.1
    """, {'cutoff': cutoff_datetime})


async def query_all_entities_with_embedding(db: AsyncSurreal) -> QueryResult:
    """Get all entities with their embeddings for similarity comparison."""
    return await run_query(db, "SELECT id, content, embedding, access_count, accessed FROM entity")


async def query_similar_by_embedding(
    db: AsyncSurreal,
    embedding: list[float],
    exclude_id: str,
    limit: int = 10
) -> QueryResult:
    """Find similar entities by embedding with similarity score."""
    return await run_query(db, f"""
        SELECT id, content, access_count, accessed,
               vector::similarity::cosine(embedding, $emb) AS sim
        FROM entity
        WHERE embedding <|{limit},40|> $emb AND id != $exclude_rec
    """, {'emb': embedding, 'exclude_rec': RecordID('entity', _extract_id(exclude_id))})


async def query_delete_entity_by_record_id(db: AsyncSurreal, entity_id: str) -> QueryResult:
    """Delete entity by record ID (used in reflect merge).

    Note: SurrealDB automatically cleans up relations when the record is deleted.
    """
    return await run_query(db, """
        DELETE type::record("entity", $id)
    """, {'id': entity_id})


async def query_entity_with_embedding(db: AsyncSurreal, entity_id: str) -> QueryResult:
    """Get entity with embedding for contradiction checking."""
    return await run_query(db, 'SELECT * FROM type::record("entity", $id)', {'id': entity_id})


async def query_similar_for_contradiction(
    db: AsyncSurreal,
    embedding: list[float],
    entity_id: str
) -> QueryResult:
    """Find similar entities for contradiction detection."""
    return await run_query(db, """
        SELECT id, content FROM entity
        WHERE embedding <|10,40|> $emb AND id != $exclude_rec
    """, {'emb': embedding, 'exclude_rec': RecordID('entity', entity_id)})


async def query_entities_by_labels(db: AsyncSurreal, labels: list[str]) -> QueryResult:
    """Get entities filtered by labels."""
    label_filter = "WHERE labels CONTAINSANY $labels" if labels else ""
    return await run_query(db, f"SELECT id, content, embedding FROM entity {label_filter}", {'labels': labels})


async def query_vector_similarity(db: AsyncSurreal, emb1: list[float], emb2: list[float]) -> QueryResult:
    """Calculate cosine similarity between two embeddings."""
    # v3.0: SELECT requires FROM, use RETURN for computed values
    # Wrap in list for consistent QueryResult format
    result = await run_query(db, """
        RETURN { sim: vector::similarity::cosine($emb1, $emb2) }
    """, {'emb1': emb1, 'emb2': emb2})
    # RETURN gives dict directly, wrap in list
    if isinstance(result, dict):
        return [result]
    return result


async def query_count_entities(db: AsyncSurreal, context: str | None = None) -> QueryResult:
    """Count entities, optionally filtered by context."""
    context_filter = "WHERE context = $context" if context else ""
    return await run_query(db, f"SELECT count() FROM entity {context_filter} GROUP ALL", {'context': context})


async def query_count_relations(db: AsyncSurreal, context: str | None = None) -> QueryResult:
    """Count relations, optionally filtered by context (via source entity)."""
    if context:
        return await run_query(db, """
            SELECT count() FROM relates WHERE in.context = $context GROUP ALL
        """, {'context': context})
    return await run_query(db, "SELECT count() FROM relates GROUP ALL")


async def query_get_all_labels(db: AsyncSurreal) -> QueryResult:
    """Get all unique labels."""
    # v3.0: Subquery with .field syntax changed, use Python-side processing
    result = await run_query(db, "SELECT labels FROM entity")
    if not result:
        return [{'labels': []}]
    all_labels = []
    for entity in result:
        if isinstance(entity, dict) and 'labels' in entity and entity['labels']:
            all_labels.extend(entity['labels'])
    return [{'labels': list(set(all_labels))}]


async def query_count_by_label(db: AsyncSurreal, label: str) -> QueryResult:
    """Count entities with a specific label."""
    return await run_query(db, """
        SELECT count() FROM entity WHERE labels CONTAINS $label GROUP ALL
    """, {'label': label})


# =============================================================================
# Episode Query Functions (Episodic Memory)
# =============================================================================

async def query_create_episode(
    db: AsyncSurreal,
    episode_id: str,
    content: str,
    embedding: list[float],
    timestamp: str,
    summary: str | None,
    metadata: dict[str, Any] | None,
    context: str | None
) -> QueryResult:
    """Create or update an episode."""
    # Handle timestamp: append Z if missing timezone for SurrealDB datetime cast
    # SurrealDB requires ISO8601 with timezone (e.g., 2026-01-06T12:00:00Z)
    ts_with_tz = timestamp
    if timestamp and not (timestamp.endswith('Z') or '+' in timestamp or timestamp.endswith('00:00')):
        ts_with_tz = timestamp + 'Z'

    return await run_query(db, """
        UPSERT type::record("episode", $id) SET
            content = $content,
            embedding = $embedding,
            timestamp = IF $timestamp THEN <datetime>$timestamp ELSE time::now() END,
            summary = $summary,
            metadata = $metadata,
            context = $context
    """, {
        'id': episode_id,
        'content': content,
        'embedding': embedding,
        'timestamp': ts_with_tz,
        'summary': summary,
        'metadata': metadata or {},
        'context': context
    })


async def query_search_episodes(
    db: AsyncSurreal,
    query: str,
    query_embedding: list[float],
    time_start: str | None,
    time_end: str | None,
    context: str | None,
    limit: int
) -> QueryResult:
    """Hybrid search for episodes with temporal filtering."""
    # Handle timestamp format: append Z if missing timezone for SurrealDB
    def fix_timestamp(ts: str | None) -> str | None:
        if ts and not (ts.endswith('Z') or '+' in ts or ts.endswith('00:00')):
            return ts + 'Z'
        return ts

    time_start_fixed = fix_timestamp(time_start)
    time_end_fixed = fix_timestamp(time_end)

    time_filter = ""
    if time_start_fixed:
        time_filter += " AND timestamp >= <datetime>$time_start"
    if time_end_fixed:
        time_filter += " AND timestamp <= <datetime>$time_end"
    context_filter = " AND context = $context" if context else ""

    # Use search::rrf() to combine BM25 and vector search results
    # Using inline subqueries (LET variables don't work with search::rrf)
    return await run_query(db, f"""
        SELECT * FROM search::rrf([
            (SELECT id, content, summary, timestamp, metadata, context
             FROM episode
             WHERE embedding <|{limit * 2},40|> $emb {time_filter} {context_filter}),
            (SELECT id, content, summary, timestamp, metadata, context
             FROM episode
             WHERE content @0@ $q {time_filter} {context_filter})
        ], $limit, 60)
    """, {
        'q': query,
        'emb': query_embedding,
        'time_start': time_start_fixed,
        'time_end': time_end_fixed,
        'context': context,
        'limit': limit
    })


async def query_get_episode(db: AsyncSurreal, episode_id: str) -> QueryResult:
    """Get episode by ID."""
    return await run_query(db, """
        SELECT * FROM type::record("episode", $id)
    """, {'id': episode_id})


async def query_update_episode_access(db: AsyncSurreal, episode_id: str) -> QueryResult:
    """Update episode access timestamp and count."""
    return await run_query(db, """
        UPDATE type::record("episode", $id) SET accessed = time::now(), access_count += 1
    """, {'id': episode_id})


async def query_link_entity_to_episode(
    db: AsyncSurreal,
    entity_id: str,
    episode_id: str,
    position: int | None,
    confidence: float
) -> QueryResult:
    """Link an entity to its source episode."""
    return await run_query(db, """
        RELATE $entity_rec->extracted_from->$episode_rec
        SET position = $position, confidence = $confidence
    """, {
        'entity_rec': RecordID('entity', entity_id),
        'episode_rec': RecordID('episode', episode_id),
        'position': position,
        'confidence': confidence
    })


async def query_get_episode_entities(db: AsyncSurreal, episode_id: str) -> QueryResult:
    """Get all entities extracted from an episode."""
    return await run_query(db, """
        SELECT <-extracted_from<-entity.* AS entities FROM type::record("episode", $id)
    """, {'id': episode_id})


async def query_delete_episode(db: AsyncSurreal, episode_id: str) -> QueryResult:
    """Delete episode and its entity links."""
    return await run_query(db, """
        DELETE type::record("episode", $id)
    """, {'id': episode_id})


async def query_count_episodes(db: AsyncSurreal, context: str | None = None) -> QueryResult:
    """Count episodes, optionally filtered by context."""
    context_filter = "WHERE context = $context" if context else ""
    return await run_query(db, f"""
        SELECT count() FROM episode {context_filter} GROUP ALL
    """, {'context': context})


# =============================================================================
# Importance Scoring Functions
# =============================================================================

async def query_get_entity_connectivity(db: AsyncSurreal, entity_id: str) -> int:
    """Count total relations (in + out) for an entity."""
    result = await run_query(db, """
        LET $out_count = (SELECT count() FROM relates WHERE in = $rec GROUP ALL);
        LET $in_count = (SELECT count() FROM relates WHERE out = $rec GROUP ALL);
        RETURN {
            outgoing: $out_count[0].count ?? 0,
            incoming: $in_count[0].count ?? 0
        };
    """, {'rec': RecordID('entity', entity_id)})

    if result and isinstance(result, dict):
        return result.get('outgoing', 0) + result.get('incoming', 0)
    if result and isinstance(result, list) and len(result) > 0:
        r = result[0]
        if isinstance(r, dict):
            return r.get('outgoing', 0) + r.get('incoming', 0)
    return 0


async def query_recalculate_importance(db: AsyncSurreal, entity_id: str) -> float:
    """Recalculate importance score for an entity.

    Formula: importance = 0.3*connectivity_score + 0.3*access_score + 0.4*user_importance

    Where:
    - connectivity_score = min(1.0, relation_count / 10)
    - access_score = min(1.0, log10(access_count + 1) / 3)
    - user_importance = user-set value or 0.5 default
    """
    import math

    entity_result = await query_get_entity(db, entity_id)
    if not entity_result:
        return 0.5

    e = entity_result[0]
    access_count = e.get('access_count', 0)
    user_imp = e.get('user_importance')

    # Get connectivity
    connectivity = await query_get_entity_connectivity(db, entity_id)

    # Calculate scores
    connectivity_score = min(1.0, connectivity / 10.0)
    access_score = min(1.0, math.log10(access_count + 1) / 3.0)
    user_score = user_imp if user_imp is not None else 0.5

    # Weighted combination
    importance = 0.3 * connectivity_score + 0.3 * access_score + 0.4 * user_score

    # Update entity
    await run_query(db, """
        UPDATE type::record("entity", $id) SET importance = $importance
    """, {'id': entity_id, 'importance': importance})

    return importance


async def query_batch_recalculate_importance(db: AsyncSurreal, context: str | None = None) -> int:
    """Recalculate importance for all entities (optionally filtered by context)."""
    context_filter = "WHERE context = $ctx" if context else ""
    entities = await run_query(db, f"SELECT id FROM entity {context_filter}", {'ctx': context})

    count = 0
    for e in entities:
        await query_recalculate_importance(db, _extract_id(e['id']))
        count += 1

    return count


async def query_weighted_search(
    db: AsyncSurreal,
    query: str,
    query_embedding: list[float],
    labels: list[str],
    limit: int,
    context: str | None = None,
    importance_weight: float = 0.2,
    recency_weight: float = 0.1
) -> QueryResult:
    """Hybrid search weighted by importance and recency.

    Final ranking considers:
    - RRF score from hybrid search (70%)
    - Entity importance (20%)
    - Recency of access (10%)
    """
    label_filter = "AND labels CONTAINSANY $labels" if labels else ""
    context_filter = "AND context = $context" if context else ""

    # First do RRF search, then re-rank with importance/recency
    # Note: SurrealDB v3.0 doesn't support arithmetic in ORDER BY easily,
    # so we fetch more results and let Python re-rank if needed
    return await run_query(db, f"""
        LET $ft = SELECT id, type, labels, content, confidence, source, decay_weight,
                         context, importance, accessed
                  FROM entity
                  WHERE content @@ $q {label_filter} {context_filter};

        LET $vs = SELECT id, type, labels, content, confidence, source, decay_weight,
                         context, importance, accessed
                  FROM entity
                  WHERE embedding <|{limit * 3},40|> $emb {label_filter} {context_filter};

        RETURN search::rrf([$vs, $ft], $limit, 60);
    """, {
        'q': query,
        'emb': query_embedding,
        'labels': labels,
        'context': context,
        'limit': limit
    })


# =============================================================================
# Context Management Functions
# =============================================================================

async def query_list_contexts(db: AsyncSurreal) -> QueryResult:
    """List all unique contexts from entities and episodes."""
    # Get contexts from both entities and episodes using GROUP BY (DISTINCT deprecated in v3.0)
    entity_contexts = await run_query(db, """
        SELECT context FROM entity WHERE context != NONE GROUP BY context
    """)
    episode_contexts = await run_query(db, """
        SELECT context FROM episode WHERE context != NONE GROUP BY context
    """)

    # Combine and deduplicate
    all_contexts = set()
    for r in entity_contexts:
        if isinstance(r, dict) and r.get('context'):
            all_contexts.add(r['context'])
    for r in episode_contexts:
        if isinstance(r, dict) and r.get('context'):
            all_contexts.add(r['context'])

    return [{'contexts': list(all_contexts)}]


async def query_get_context_stats(db: AsyncSurreal, context: str) -> QueryResult:
    """Get statistics for a specific context."""
    entity_count = await run_query(db, """
        SELECT count() FROM entity WHERE context = $ctx GROUP ALL
    """, {'ctx': context})

    episode_count = await run_query(db, """
        SELECT count() FROM episode WHERE context = $ctx GROUP ALL
    """, {'ctx': context})

    relation_count = await run_query(db, """
        SELECT count() FROM relates
        WHERE in.context = $ctx OR out.context = $ctx
        GROUP ALL
    """, {'ctx': context})

    return [{
        'context': context,
        'entities': entity_count[0].get('count', 0) if entity_count else 0,
        'episodes': episode_count[0].get('count', 0) if episode_count else 0,
        'relations': relation_count[0].get('count', 0) if relation_count else 0
    }]


# =============================================================================
# Entity Type Query Functions
# =============================================================================

async def query_entities_by_type(
    db: AsyncSurreal,
    entity_type: str,
    context: str | None = None,
    limit: int = 100
) -> QueryResult:
    """Get all entities of a specific type."""
    context_filter = "AND context = $context" if context else ""
    return await run_query(db, f"""
        SELECT * FROM entity WHERE type = $type {context_filter} LIMIT $limit
    """, {'type': entity_type, 'context': context, 'limit': limit})


async def query_count_by_type(db: AsyncSurreal, entity_type: str, context: str | None = None) -> QueryResult:
    """Count entities of a specific type."""
    context_filter = "AND context = $context" if context else ""
    return await run_query(db, f"""
        SELECT count() FROM entity WHERE type = $type {context_filter} GROUP ALL
    """, {'type': entity_type, 'context': context})


async def query_list_types(db: AsyncSurreal, context: str | None = None) -> QueryResult:
    """List all entity types in use with counts."""
    context_filter = "WHERE context = $context" if context else ""
    result = await run_query(db, f"""
        SELECT type, count() AS count FROM entity {context_filter} GROUP BY type
    """, {'context': context})
    return result


# =============================================================================
# Procedure Query Functions (Procedural Memory)
# =============================================================================

async def query_create_procedure(
    db: AsyncSurreal,
    procedure_id: str,
    name: str,
    description: str,
    steps: list[dict[str, Any]],
    embedding: list[float],
    context: str | None,
    labels: list[str]
) -> QueryResult:
    """Create or update a procedure."""
    # TODO: Use <set<string>>$labels when Python SDK supports CBOR tag 56
    return await run_query(db, """
        UPSERT type::record("procedure", $id) SET
            name = $name,
            description = $description,
            steps = $steps,
            embedding = $embedding,
            context = $context,
            labels = array::distinct($labels)
    """, {
        'id': procedure_id,
        'name': name,
        'description': description,
        'steps': steps,
        'embedding': embedding,
        'context': context,
        'labels': labels
    })


async def query_get_procedure(db: AsyncSurreal, procedure_id: str) -> QueryResult:
    """Get procedure by ID."""
    return await run_query(db, """
        SELECT * FROM type::record("procedure", $id)
    """, {'id': procedure_id})


async def query_search_procedures(
    db: AsyncSurreal,
    query: str,
    query_embedding: list[float],
    context: str | None,
    labels: list[str],
    limit: int
) -> QueryResult:
    """Hybrid search for procedures."""
    context_filter = "AND context = $context" if context else ""
    label_filter = "AND labels CONTAINSANY $labels" if labels else ""

    # Use search::rrf() to combine BM25 and vector search results
    # Using inline subqueries (LET variables don't work with search::rrf)
    return await run_query(db, f"""
        SELECT * FROM search::rrf([
            (SELECT id, name, description, steps, context, labels
             FROM procedure
             WHERE embedding <|{limit * 2},40|> $emb {context_filter} {label_filter}),
            (SELECT id, name, description, steps, context, labels
             FROM procedure
             WHERE name @0@ $q OR description @1@ $q {context_filter} {label_filter})
        ], $limit, 60)
    """, {
        'q': query,
        'emb': query_embedding,
        'context': context,
        'labels': labels,
        'limit': limit
    })


async def query_update_procedure_access(db: AsyncSurreal, procedure_id: str) -> QueryResult:
    """Update procedure access timestamp and count."""
    return await run_query(db, """
        UPDATE type::record("procedure", $id) SET accessed = time::now(), access_count += 1
    """, {'id': procedure_id})


async def query_delete_procedure(db: AsyncSurreal, procedure_id: str) -> QueryResult:
    """Delete a procedure."""
    return await run_query(db, """
        DELETE type::record("procedure", $id)
    """, {'id': procedure_id})


async def query_list_procedures(db: AsyncSurreal, context: str | None = None, limit: int = 100) -> QueryResult:
    """List all procedures, optionally filtered by context."""
    context_filter = "WHERE context = $context" if context else ""
    return await run_query(db, f"""
        SELECT id, name, description, array::len(steps) AS step_count, context, labels, accessed
        FROM procedure {context_filter}
        ORDER BY accessed DESC
        LIMIT $limit
    """, {'context': context, 'limit': limit})


async def query_count_procedures(db: AsyncSurreal, context: str | None = None) -> QueryResult:
    """Count procedures, optionally filtered by context."""
    context_filter = "WHERE context = $context" if context else ""
    return await run_query(db, f"""
        SELECT count() FROM procedure {context_filter} GROUP ALL
    """, {'context': context})


__all__ = [
    'AppContext',
    'app_lifespan',
    'get_db',
    'run_query',
    'DBConnection',
    'validate_entity',
    'validate_relation',
    'QueryResult',
    'SURREALDB_URL',
    'SURREALDB_NAMESPACE',
    'SURREALDB_DATABASE',
    'SCHEMA_SQL',
    # Context detection
    'detect_context',
    'MEMCP_DEFAULT_CONTEXT',
    'MEMCP_CONTEXT_FROM_CWD',
    # Entity type ontology
    'ENTITY_TYPES',
    'ALLOW_CUSTOM_TYPES',
    'validate_entity_type',
    'get_entity_types',
    # Entity query functions
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
    # Entity type query functions
    'query_entities_by_type',
    'query_count_by_type',
    'query_list_types',
    # Episode query functions
    'query_create_episode',
    'query_search_episodes',
    'query_get_episode',
    'query_update_episode_access',
    'query_link_entity_to_episode',
    'query_get_episode_entities',
    'query_delete_episode',
    'query_count_episodes',
    # Importance scoring functions
    'query_get_entity_connectivity',
    'query_recalculate_importance',
    'query_batch_recalculate_importance',
    'query_weighted_search',
    # Context management functions
    'query_list_contexts',
    'query_get_context_stats',
    # Procedure query functions
    'query_create_procedure',
    'query_get_procedure',
    'query_search_procedures',
    'query_update_procedure_access',
    'query_delete_procedure',
    'query_list_procedures',
    'query_count_procedures',
]
