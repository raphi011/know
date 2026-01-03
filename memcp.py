import json
import os
from datetime import datetime, timedelta
from mcp.server.fastmcp import FastMCP
from surrealdb import AsyncSurreal
from sentence_transformers import SentenceTransformer, CrossEncoder

# Configuration from environment
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")
SURREALDB_NAMESPACE = os.getenv("SURREALDB_NAMESPACE", "knowledge")
SURREALDB_DATABASE = os.getenv("SURREALDB_DATABASE", "graph")
SURREALDB_USER = os.getenv("SURREALDB_USER")
SURREALDB_PASS = os.getenv("SURREALDB_PASS")

mcp = FastMCP("knowledge-graph")
db = AsyncSurreal(SURREALDB_URL)
embedder = SentenceTransformer('all-MiniLM-L6-v2')
nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')

# NLI labels: contradiction, entailment, neutral
NLI_LABELS = ['contradiction', 'entailment', 'neutral']

_initialized = False


async def ensure_init():
    global _initialized
    if _initialized:
        return

    if SURREALDB_USER and SURREALDB_PASS:
        await db.signin({
            "namespace": SURREALDB_NAMESPACE,
            "database": SURREALDB_DATABASE,
            "username": SURREALDB_USER,
            "password": SURREALDB_PASS
        })

    await db.use(SURREALDB_NAMESPACE, SURREALDB_DATABASE)

    # Schema setup with new fields
    await db.query("""
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
        DEFINE INDEX entity_embedding ON entity FIELDS embedding MTREE DIMENSION 384 DIST COSINE;

        -- Full-text search index with BM25
        DEFINE ANALYZER entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
        DEFINE INDEX entity_content_ft ON entity FIELDS content SEARCH ANALYZER entity_analyzer BM25;
    """)
    _initialized = True


def embed(text: str) -> list[float]:
    return embedder.encode(text).tolist()


def check_contradiction(text1: str, text2: str) -> dict:
    """Check if two texts contradict each other using NLI."""
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
    await ensure_init()

    query_embedding = embed(query)
    labels = labels or []

    label_filter = "AND labels CONTAINSANY $labels" if labels else ""

    # Hybrid search using SurrealDB's native capabilities
    results = await db.query(f"""
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

    # Update access metadata
    entities = results[0] if results and results[0] else []
    for r in entities:
        await db.query("""
            UPDATE $id SET accessed = time::now(), access_count += 1
        """, {'id': r['id']})

    return json.dumps(entities, indent=2, default=str)


@mcp.tool()
async def get_entity(id: str) -> str:
    """Retrieve a specific memory by its ID. Use when you have an exact entity ID from a previous search or traversal."""
    await ensure_init()

    results = await db.query("""
        SELECT * FROM type::thing("entity", $id)
    """, {'id': id})

    entity = results[0][0] if results and results[0] else None
    if entity:
        await db.query("""
            UPDATE type::thing("entity", $id) SET accessed = time::now(), access_count += 1
        """, {'id': id})

    return json.dumps(entity, indent=2, default=str)


@mcp.tool()
async def list_labels() -> str:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    await ensure_init()

    results = await db.query("""
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
    await ensure_init()

    relation_types = relation_types or []

    if relation_types:
        # Filtered traversal by relation type
        type_filter = '|'.join(relation_types)
        results = await db.query(f"""
            SELECT *, ->(({type_filter}))..{depth}->entity AS connected
            FROM type::thing("entity", $id)
        """, {'id': start})
    else:
        results = await db.query(f"""
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
    await ensure_init()

    # Use SurrealDB 2.2+ native shortest path
    results = await db.query(f"""
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
    await ensure_init()

    entities = entities or []
    relations = relations or []
    stored = {"entities": 0, "relations": 0, "contradictions": []}

    for entity in entities:
        # Check for contradictions if requested
        if check_contradictions:
            similar = await db.query("""
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

        await db.query("""
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
        await db.query("""
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
        msg += f"\n⚠️ Found {len(stored['contradictions'])} potential contradictions:\n"
        msg += json.dumps(stored['contradictions'], indent=2)

    return msg


@mcp.tool()
async def forget(id: str) -> str:
    """Delete information from persistent memory. Use when the user says 'forget this', 'remove...', 'delete...', or when information is explicitly outdated or wrong."""
    await ensure_init()

    await db.query("""
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
    await ensure_init()

    report = {"decayed": 0, "similar_pairs": []}

    # Apply temporal decay
    if apply_decay:
        cutoff = datetime.now() - timedelta(days=decay_days)
        result = await db.query("""
            UPDATE entity SET decay_weight = decay_weight * 0.9
            WHERE accessed < $cutoff AND decay_weight > 0.1
        """, {'cutoff': cutoff.isoformat()})
        report['decayed'] = len(result[0]) if result and result[0] else 0

    # Find similar entities that might be duplicates
    if find_similar:
        entities = await db.query("SELECT id, content, embedding FROM entity")
        entities = entities[0] if entities else []

        for i, e1 in enumerate(entities):
            for e2 in entities[i+1:]:
                similar = await db.query("""
                    SELECT vector::similarity::cosine($emb1, $emb2) AS sim
                """, {'emb1': e1['embedding'], 'emb2': e2['embedding']})

                sim = similar[0][0]['sim'] if similar and similar[0] else 0
                if sim >= similarity_threshold:
                    report['similar_pairs'].append({
                        'entity1': {'id': e1['id'], 'content': e1['content'][:100]},
                        'entity2': {'id': e2['id'], 'content': e2['content'][:100]},
                        'similarity': sim
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
        # Check specific entity against similar ones
        entity = await db.query('SELECT * FROM type::thing("entity", $id)', {'id': entity_id})
        entity = entity[0][0] if entity and entity[0] else None

        if entity:
            similar = await db.query("""
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
        # Check within labels or all entities
        label_filter = "WHERE labels CONTAINSANY $labels" if labels else ""
        entities = await db.query(f"SELECT id, content, embedding FROM entity {label_filter}", {'labels': labels})
        entities = entities[0] if entities else []

        # Compare semantically similar pairs
        for i, e1 in enumerate(entities):
            for e2 in entities[i+1:]:
                # Only check pairs that are somewhat similar
                similar = await db.query("""
                    SELECT vector::similarity::cosine($emb1, $emb2) AS sim
                """, {'emb1': e1['embedding'], 'emb2': e2['embedding']})

                sim = similar[0][0]['sim'] if similar and similar[0] else 0
                if sim > 0.5:  # Only check semantically related content
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
