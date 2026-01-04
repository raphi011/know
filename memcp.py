import asyncio
import json
import logging
import os
from datetime import datetime, timedelta
from typing import Any, cast

from mcp.server.fastmcp import FastMCP

# Logging setup
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("memcp")

from surrealdb import AsyncSurreal
from sentence_transformers import SentenceTransformer, CrossEncoder

# Type alias for query results - SurrealDB returns list[Value] but we know
# our queries return list of dicts in practice
QueryResult = list[list[dict[str, Any]]]

# Configuration from environment
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")
SURREALDB_NAMESPACE = os.getenv("SURREALDB_NAMESPACE", "knowledge")
SURREALDB_DATABASE = os.getenv("SURREALDB_DATABASE", "graph")
SURREALDB_USER = os.getenv("SURREALDB_USER")
SURREALDB_PASS = os.getenv("SURREALDB_PASS")

mcp = FastMCP("knowledge-graph")
db = AsyncSurreal(SURREALDB_URL)

# Embedding model: Transforms text into 384-dimensional vectors where semantically
# similar texts cluster together in vector space. "all-MiniLM-L6-v2" is a lightweight
# model trained on 1B+ sentence pairs - good balance of speed vs quality.
embedder = SentenceTransformer('all-MiniLM-L6-v2')

# NLI (Natural Language Inference) model: Given two sentences, classifies their
# relationship as contradiction/entailment/neutral. Uses DeBERTa architecture
# fine-tuned on SNLI+MNLI datasets. CrossEncoder means both sentences are processed
# together (vs bi-encoder which encodes separately) - slower but more accurate.
nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')

# NLI output labels:
# - contradiction: statements cannot both be true ("sky is blue" vs "sky is green")
# - entailment: first statement implies second ("dog is running" -> "animal is moving")
# - neutral: statements are unrelated or compatible but independent
NLI_LABELS = ['contradiction', 'entailment', 'neutral']

_initialized = False

# Query timeout in seconds
QUERY_TIMEOUT = float(os.getenv("MEMCP_QUERY_TIMEOUT", "30"))


class ValidationError(Exception):
    """Raised when input validation fails."""
    pass


class DatabaseError(Exception):
    """Raised when database operations fail."""
    pass


def validate_entity(entity: dict) -> None:
    """Validate entity structure before storing."""
    if not isinstance(entity, dict):
        raise ValidationError(f"Entity must be a dict, got {type(entity).__name__}")
    if "id" not in entity:
        raise ValidationError("Entity missing required field: 'id'")
    if "content" not in entity:
        raise ValidationError("Entity missing required field: 'content'")
    if not isinstance(entity["id"], str) or not entity["id"].strip():
        raise ValidationError("Entity 'id' must be a non-empty string")
    if not isinstance(entity["content"], str) or not entity["content"].strip():
        raise ValidationError("Entity 'content' must be a non-empty string")
    if "labels" in entity and not isinstance(entity.get("labels"), list):
        raise ValidationError("Entity 'labels' must be a list")
    if "confidence" in entity:
        conf = entity["confidence"]
        if not isinstance(conf, (int, float)) or not 0 <= conf <= 1:
            raise ValidationError("Entity 'confidence' must be a number between 0 and 1")


def validate_relation(relation: dict) -> None:
    """Validate relation structure before storing."""
    if not isinstance(relation, dict):
        raise ValidationError(f"Relation must be a dict, got {type(relation).__name__}")
    for field in ("from", "to", "type"):
        if field not in relation:
            raise ValidationError(f"Relation missing required field: '{field}'")
        if not isinstance(relation[field], str) or not relation[field].strip():
            raise ValidationError(f"Relation '{field}' must be a non-empty string")
    if "weight" in relation:
        weight = relation["weight"]
        if not isinstance(weight, (int, float)):
            raise ValidationError("Relation 'weight' must be a number")


async def _query(sql: str, vars: dict[str, Any] | None = None) -> QueryResult:
    """Typed wrapper around db.query() with timeout and error handling."""
    try:
        async with asyncio.timeout(QUERY_TIMEOUT):
            result = await db.query(sql, cast(Any, vars))
            return cast(QueryResult, result)
    except asyncio.TimeoutError:
        logger.error(f"Query timed out after {QUERY_TIMEOUT}s")
        raise DatabaseError(f"Query timed out after {QUERY_TIMEOUT}s")
    except Exception as e:
        logger.error(f"Database query failed: {e}")
        raise DatabaseError(f"Database query failed: {e}") from e


async def ensure_init():
    """Lazy init: schema creation happens on first tool call, not import."""
    global _initialized
    if _initialized:
        return

    try:
        async with asyncio.timeout(QUERY_TIMEOUT):
            await db.connect()

            if SURREALDB_USER and SURREALDB_PASS:
                await db.signin({
                    "namespace": SURREALDB_NAMESPACE,
                    "database": SURREALDB_DATABASE,
                    "username": SURREALDB_USER,
                    "password": SURREALDB_PASS
                })

            await db.use(SURREALDB_NAMESPACE, SURREALDB_DATABASE)
    except asyncio.TimeoutError:
        logger.error(f"Database connection timed out after {QUERY_TIMEOUT}s")
        raise DatabaseError(f"Could not connect to SurrealDB at {SURREALDB_URL}: connection timed out")
    except Exception as e:
        logger.error(f"Database connection failed: {e}")
        raise DatabaseError(f"Could not connect to SurrealDB at {SURREALDB_URL}: {e}") from e

    logger.info(f"Connected to SurrealDB at {SURREALDB_URL}")

    # Schema: each entity stores content + its vector embedding for similarity search.
    # decay_weight enables "forgetting" - memories accessed less often fade over time.
    # access_count tracks retrieval frequency for importance scoring.
    await _query("""
        DEFINE TABLE entity SCHEMAFULL;
        DEFINE FIELD type ON entity TYPE string;
        DEFINE FIELD labels ON entity TYPE array<string>;
        DEFINE FIELD content ON entity TYPE string;
        DEFINE FIELD embedding ON entity TYPE array<float>;
        DEFINE FIELD confidence ON entity TYPE float DEFAULT 1.0;
        DEFINE FIELD source ON entity TYPE string;
        DEFINE FIELD decay_weight ON entity TYPE float DEFAULT 1.0;
        DEFINE FIELD created ON entity TYPE datetime DEFAULT time::now();
        DEFINE FIELD accessed ON entity TYPE datetime DEFAULT time::now();
        DEFINE FIELD access_count ON entity TYPE int DEFAULT 0;

        DEFINE INDEX entity_labels ON entity FIELDS labels;

        -- MTREE index: data structure optimized for approximate nearest neighbor (ANN)
        -- search in high-dimensional spaces. DIMENSION 384 matches our embedding size.
        -- COSINE distance: measures angle between vectors (1 = identical direction,
        -- 0 = orthogonal, -1 = opposite). Preferred over Euclidean for text because
        -- it's magnitude-invariant - "dog" and "DOG DOG DOG" point same direction.
        DEFINE INDEX entity_embedding ON entity FIELDS embedding MTREE DIMENSION 384 DIST COSINE;

        -- BM25 (Best Match 25): probabilistic ranking function for keyword search.
        -- Improves on TF-IDF by adding document length normalization and term
        -- saturation (diminishing returns for repeated terms). Snowball stemmer
        -- reduces words to roots ("running" -> "run") for better recall.
        DEFINE ANALYZER entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
        DEFINE INDEX entity_content_ft ON entity FIELDS content SEARCH ANALYZER entity_analyzer BM25;
    """)
    _initialized = True


def embed(text: str) -> list[float]:
    """
    Convert text to dense vector representation. The model maps semantically
    similar texts to nearby points in 384-dim space. This enables "fuzzy" search -
    "canine" matches "dog" even though they share no characters.
    """
    return embedder.encode(text).tolist()


def check_contradiction(text1: str, text2: str) -> dict:
    """
    Use NLI to detect logical conflicts between statements. The model outputs
    logits for each class; we return both the winning label and raw scores
    (useful for thresholding - e.g., only flag if contradiction score > 0.8).
    """
    scores = nli_model.predict([(text1, text2)])
    label_idx = scores.argmax()
    return {
        'label': NLI_LABELS[label_idx],
        'scores': {NLI_LABELS[i]: float(scores[i]) for i in range(3)}
    }


@mcp.tool()
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    semantic_weight: float = 0.5
) -> str:
    """Search your persistent memory for previously stored knowledge. Use when the user asks 'do you remember...', 'what do you know about...', 'recall...', or needs context from past conversations. Combines semantic similarity and keyword matching."""
    if not query or not query.strip():
        return "Error: Query cannot be empty"
    if limit < 1 or limit > 100:
        return "Error: Limit must be between 1 and 100"
    if not 0 <= semantic_weight <= 1:
        return "Error: semantic_weight must be between 0 and 1"

    await ensure_init()

    query_embedding = embed(query)
    labels = labels or []

    label_filter = "AND labels CONTAINSANY $labels" if labels else ""

    # Hybrid search combines two complementary approaches:
    # 1. BM25 (@1@): exact keyword matching - good for names, IDs, specific terms
    # 2. Vector similarity (<|K,EF|>): semantic matching - good for concepts, paraphrases
    # The semantic_weight param blends them: 0.0 = pure keyword, 1.0 = pure semantic.
    # <|K,EF|> syntax: K=candidates to consider, EF=expansion factor for ANN accuracy.
    results = await _query(f"""
        SELECT id, type, labels, content, confidence, source, decay_weight,
               search::score(1) AS bm25_score,
               vector::similarity::cosine(embedding, $emb) AS vec_score
        FROM entity
        WHERE (content @1@ $query OR embedding <|{limit * 2},100|> $emb) {label_filter}
        ORDER BY (vec_score * $sem_weight + search::score(1) * (1 - $sem_weight)) DESC
        LIMIT $limit
    """, {
        'query': query,
        'emb': query_embedding,
        'labels': labels,
        'limit': limit,
        'sem_weight': semantic_weight
    })

    # Track access patterns - used by reflect() to identify stale memories
    entities = results[0] if results and results[0] else []
    for r in entities:
        await _query("""
            UPDATE $id SET accessed = time::now(), access_count += 1
        """, {'id': r['id']})

    return json.dumps(entities, indent=2, default=str)


@mcp.tool()
async def get_entity(id: str) -> str:
    """Retrieve a specific memory by its ID. Use when you have an exact entity ID from a previous search or traversal."""
    if not id or not id.strip():
        return "Error: ID cannot be empty"

    await ensure_init()

    # type::thing() constructs a record ID from table name + id string
    results = await _query("""
        SELECT * FROM type::thing("entity", $id)
    """, {'id': id})

    entity = results[0][0] if results and results[0] else None
    if entity:
        await _query("""
            UPDATE type::thing("entity", $id) SET accessed = time::now(), access_count += 1
        """, {'id': id})

    return json.dumps(entity, indent=2, default=str)


@mcp.tool()
async def list_labels() -> str:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    await ensure_init()

    # Flatten all label arrays into one, then deduplicate
    results = await _query("""
        SELECT array::distinct(array::flatten((SELECT labels FROM entity))) AS labels
    """)
    labels = results[0][0]['labels'] if results and results[0] else []
    return json.dumps(labels, indent=2)


@mcp.tool()
async def traverse(
    start: str,
    depth: int = 2,
    relation_types: list[str] | None = None
) -> str:
    """Explore how stored knowledge connects to other knowledge. Use when the user asks 'what's related to...', 'how does X connect to Y', or wants to understand context around a topic."""
    if not start or not start.strip():
        return "Error: start ID cannot be empty"
    if depth < 1 or depth > 10:
        return "Error: depth must be between 1 and 10"

    await ensure_init()

    relation_types = relation_types or []

    # SurrealDB graph traversal syntax:
    # -> follows outgoing edges, ..N means up to N hops
    # ->? means any edge type, ->(type1|type2) filters to specific types
    # Returns the starting entity plus all reachable entities within depth
    if relation_types:
        type_filter = '|'.join(relation_types)
        results = await _query(f"""
            SELECT *, ->(({type_filter}))..{depth}->entity AS connected
            FROM type::thing("entity", $id)
        """, {'id': start})
    else:
        results = await _query(f"""
            SELECT *, ->?..{depth}->entity AS connected
            FROM type::thing("entity", $id)
        """, {'id': start})

    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool()
async def find_path(
    from_id: str,
    to_id: str,
    max_depth: int = 5
) -> str:
    """Find how two pieces of knowledge are connected through intermediate relationships. Use when the user asks 'how is X related to Y' or wants to trace connections between concepts."""
    if not from_id or not from_id.strip():
        return "Error: from_id cannot be empty"
    if not to_id or not to_id.strip():
        return "Error: to_id cannot be empty"
    if max_depth < 1 or max_depth > 20:
        return "Error: max_depth must be between 1 and 20"

    await ensure_init()

    # SurrealDB 2.2+ shortest path: {..N+shortest=target} finds minimum-hop path
    # between two nodes. Useful for explaining how concepts relate through
    # intermediate nodes (e.g., "how is Python related to web development?")
    results = await _query(f"""
        SELECT * FROM type::thing("entity", $from).{{..{max_depth}+shortest=type::thing("entity", $to)}}->?->entity
    """, {'from': from_id, 'to': to_id})

    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool()
async def remember(
    entities: list[dict] | None = None,
    relations: list[dict] | None = None,
    check_contradictions: bool = False
) -> str:
    """Store important information in persistent memory for future sessions. Use proactively when the user shares preferences, facts about themselves, project context, decisions, or anything they'd want you to recall later. Supports confidence scores and contradiction detection.

    entities: list of {id, content, type?, labels?, confidence?, source?}
    relations: list of {from, to, type, weight?}
    """
    entities = entities or []
    relations = relations or []

    # Validate all inputs before any DB operations
    for i, entity in enumerate(entities):
        try:
            validate_entity(entity)
        except ValidationError as e:
            return f"Error: Invalid entity at index {i}: {e}"

    for i, relation in enumerate(relations):
        try:
            validate_relation(relation)
        except ValidationError as e:
            return f"Error: Invalid relation at index {i}: {e}"

    await ensure_init()

    stored = {"entities": 0, "relations": 0, "contradictions": []}

    for entity in entities:
        if check_contradictions:
            # Find semantically similar entities using vector search, then run
            # NLI to check for logical conflicts. This catches cases like storing
            # "user prefers dark mode" when "user prefers light mode" already exists.
            similar = await _query("""
                SELECT id, content FROM entity
                WHERE embedding <|5,100|> $emb AND id != $id
            """, {
                'emb': embed(entity['content']),
                'id': f"entity:{entity['id']}"
            })

            for existing in (similar[0] if similar else []):
                result = check_contradiction(entity['content'], existing['content'])
                if result['label'] == 'contradiction':
                    stored['contradictions'].append({
                        'new': entity['content'],
                        'existing_id': existing['id'],
                        'existing': existing['content'],
                        'confidence': result['scores']['contradiction']
                    })

        # UPSERT: insert if new, update if exists. Embedding is regenerated
        # on every update to ensure it matches current content.
        await _query("""
            UPSERT type::thing("entity", $id) SET
                type = $type,
                labels = $labels,
                content = $content,
                embedding = $embedding,
                confidence = $confidence,
                source = $source
        """, {
            'id': entity['id'],
            'type': entity.get('type', 'concept'),
            'labels': entity.get('labels', []),
            'content': entity['content'],
            'embedding': embed(entity['content']),
            'confidence': entity.get('confidence', 1.0),
            'source': entity.get('source')
        })
        stored['entities'] += 1

    for rel in relations:
        # RELATE creates graph edges. The edge type becomes a table name,
        # enabling queries like "find all 'causes' relationships".
        # Weight can represent strength, certainty, or recency of relationship.
        await _query("""
            RELATE type::thing("entity", $from)->$type->type::thing("entity", $to) SET weight = $weight
        """, {
            'from': rel['from'],
            'type': rel['type'],
            'to': rel['to'],
            'weight': rel.get('weight', 1.0)
        })
        stored['relations'] += 1

    msg = f"Stored {stored['entities']} entities, {stored['relations']} relations"
    if stored['contradictions']:
        msg += f"\n Warning: Found {len(stored['contradictions'])} potential contradictions:\n"
        msg += json.dumps(stored['contradictions'], indent=2)

    return msg


@mcp.tool()
async def forget(id: str) -> str:
    """Delete information from persistent memory. Use when the user says 'forget this', 'remove...', 'delete...', or when information is explicitly outdated or wrong."""
    if not id or not id.strip():
        return "Error: ID cannot be empty"

    await ensure_init()

    # Delete entity + all edges pointing to/from it (both directions)
    # to avoid orphaned relationships in the graph
    await _query("""
        DELETE type::thing("entity", $id);
        DELETE FROM type::thing("entity", $id)->?;
        DELETE FROM ?->type::thing("entity", $id);
    """, {'id': id})

    return f"Removed {id}"


@mcp.tool()
async def reflect(
    apply_decay: bool = True,
    decay_days: int = 30,
    find_similar: bool = True,
    similarity_threshold: float = 0.85
) -> str:
    """Maintenance tool to clean up memory: decay old unused knowledge, find duplicates, identify clusters. Use periodically or when the user asks to 'clean up', 'organize', or 'consolidate' memories."""
    if decay_days < 1:
        return "Error: decay_days must be at least 1"
    if not 0 <= similarity_threshold <= 1:
        return "Error: similarity_threshold must be between 0 and 1"

    await ensure_init()

    report = {"decayed": 0, "similar_pairs": []}

    if apply_decay:
        # Temporal decay: memories not accessed recently lose "weight", making
        # them rank lower in searches. Mimics human forgetting - unused info fades.
        # Multiplier 0.9 = 10% decay per reflect call; floor at 0.1 prevents total loss.
        cutoff = datetime.now() - timedelta(days=decay_days)
        result = await _query("""
            UPDATE entity SET decay_weight = decay_weight * 0.9
            WHERE accessed < $cutoff AND decay_weight > 0.1
        """, {'cutoff': cutoff.isoformat()})
        report['decayed'] = len(result[0]) if result and result[0] else 0

    if find_similar:
        # Duplicate detection: high cosine similarity (>threshold) suggests redundant
        # entries that could be merged. Returns pairs for manual review - doesn't
        # auto-merge because slight differences may be intentional.
        entities = await _query("SELECT id, content, embedding FROM entity")
        entities = entities[0] if entities else []

        # Use MTREE vector index to find k-nearest neighbors for each entity.
        # This is O(n*k) instead of O(n²) - much faster for large knowledge bases.
        # Track seen pairs to avoid duplicates (A~B and B~A).
        seen_pairs: set[tuple[str, str]] = set()

        for entity in entities:
            # Query k-nearest neighbors using the vector index
            similar = await _query("""
                SELECT id, content, vector::similarity::cosine(embedding, $emb) AS sim
                FROM entity
                WHERE embedding <|10,100|> $emb AND id != $id
            """, {'emb': entity['embedding'], 'id': entity['id']})

            for candidate in (similar[0] if similar else []):
                if candidate['sim'] >= similarity_threshold:
                    # Create canonical pair key to avoid duplicates
                    id1, id2 = str(entity['id']), str(candidate['id'])
                    pair_key = (id1, id2) if id1 < id2 else (id2, id1)
                    if pair_key not in seen_pairs:
                        seen_pairs.add(pair_key)
                        report['similar_pairs'].append({
                            'entity1': {'id': entity['id'], 'content': entity['content'][:100]},
                            'entity2': {'id': candidate['id'], 'content': candidate['content'][:100]},
                            'similarity': candidate['sim']
                        })

    return json.dumps(report, indent=2, default=str)


@mcp.tool()
async def check_contradictions_tool(
    entity_id: str | None = None,
    labels: list[str] | None = None
) -> str:
    """Detect conflicting information in memory. Use when the user asks to verify consistency, or proactively before storing facts that might conflict with existing knowledge."""
    await ensure_init()

    labels = labels or []
    contradictions = []

    if entity_id:
        # Check one entity against its semantic neighbors
        entity = await _query('SELECT * FROM type::thing("entity", $id)', {'id': entity_id})
        entity = entity[0][0] if entity and entity[0] else None

        if entity:
            similar = await _query("""
                SELECT id, content FROM entity
                WHERE embedding <|10,100|> $emb AND id != type::thing("entity", $id)
            """, {'emb': entity['embedding'], 'id': entity_id})

            for other in (similar[0] if similar else []):
                result = check_contradiction(entity['content'], other['content'])
                if result['label'] == 'contradiction':
                    contradictions.append({
                        'entity1': {'id': entity_id, 'content': entity['content']},
                        'entity2': {'id': other['id'], 'content': other['content']},
                        'confidence': result['scores']['contradiction']
                    })
    else:
        # Check all entities (optionally filtered by label)
        label_filter = "WHERE labels CONTAINSANY $labels" if labels else ""
        entities = await _query(f"SELECT id, content, embedding FROM entity {label_filter}", {'labels': labels})
        entities = entities[0] if entities else []

        # Two-stage filter: first check semantic similarity (fast vector math),
        # then run NLI only on related pairs (expensive model inference).
        # Threshold 0.5 = moderately related - catches "sky is blue" vs "sky is green"
        # but skips "sky is blue" vs "pizza is delicious".
        for i, e1 in enumerate(entities):
            for e2 in entities[i+1:]:
                similar = await _query("""
                    SELECT vector::similarity::cosine($emb1, $emb2) AS sim
                """, {'emb1': e1['embedding'], 'emb2': e2['embedding']})

                sim = similar[0][0]['sim'] if similar and similar[0] else 0
                if sim > 0.5:
                    result = check_contradiction(e1['content'], e2['content'])
                    if result['label'] == 'contradiction':
                        contradictions.append({
                            'entity1': {'id': e1['id'], 'content': e1['content']},
                            'entity2': {'id': e2['id'], 'content': e2['content']},
                            'confidence': result['scores']['contradiction']
                        })

    return json.dumps(contradictions, indent=2)


def main():
    mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
