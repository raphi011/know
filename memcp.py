import json
import os
from datetime import datetime, timedelta
from mcp.server import Server
from mcp.server.stdio import stdio_server
from surrealdb import Surreal
from sentence_transformers import SentenceTransformer, CrossEncoder

# Configuration from environment
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")
SURREALDB_NAMESPACE = os.getenv("SURREALDB_NAMESPACE", "knowledge")
SURREALDB_DATABASE = os.getenv("SURREALDB_DATABASE", "graph")
SURREALDB_USER = os.getenv("SURREALDB_USER")
SURREALDB_PASS = os.getenv("SURREALDB_PASS")

server = Server("knowledge-graph")
db = Surreal()
embedder = SentenceTransformer('all-MiniLM-L6-v2')
nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')

# NLI labels: contradiction, entailment, neutral
NLI_LABELS = ['contradiction', 'entailment', 'neutral']


async def init():
    await db.connect(SURREALDB_URL)

    if SURREALDB_USER and SURREALDB_PASS:
        await db.signin({"user": SURREALDB_USER, "pass": SURREALDB_PASS})

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
        DEFINE INDEX entity_content_ft ON entity FIELDS content FULLTEXT ANALYZER entity_analyzer BM25;
    """)


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


@server.list_tools()
async def list_tools():
    return [
        {
            "name": "search",
            "description": "Hybrid search over knowledge graph combining semantic similarity and keyword matching",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query"},
                    "labels": {"type": "array", "items": {"type": "string"}, "description": "Filter by labels"},
                    "limit": {"type": "integer", "default": 10, "description": "Max results"},
                    "semantic_weight": {"type": "number", "default": 0.5, "description": "Weight for semantic vs keyword (0-1)"}
                },
                "required": ["query"]
            }
        },
        {
            "name": "get_entity",
            "description": "Direct lookup of an entity by ID",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "id": {"type": "string", "description": "Entity ID"}
                },
                "required": ["id"]
            }
        },
        {
            "name": "list_labels",
            "description": "List all unique labels in the knowledge graph",
            "inputSchema": {
                "type": "object",
                "properties": {}
            }
        },
        {
            "name": "traverse",
            "description": "Explore connections from an entity",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "start": {"type": "string", "description": "Starting entity ID"},
                    "depth": {"type": "integer", "default": 2, "description": "How deep to traverse"},
                    "relation_types": {"type": "array", "items": {"type": "string"}, "description": "Filter by relation types"}
                },
                "required": ["start"]
            }
        },
        {
            "name": "find_path",
            "description": "Find shortest path between two entities",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "from": {"type": "string", "description": "Starting entity ID"},
                    "to": {"type": "string", "description": "Target entity ID"},
                    "max_depth": {"type": "integer", "default": 5, "description": "Maximum path length"}
                },
                "required": ["from", "to"]
            }
        },
        {
            "name": "remember",
            "description": "Store entities and relations with optional confidence and source tracking",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "entities": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": {"type": "string"},
                                "type": {"type": "string"},
                                "labels": {"type": "array", "items": {"type": "string"}},
                                "content": {"type": "string"},
                                "confidence": {"type": "number", "description": "How certain is this fact (0-1)"},
                                "source": {"type": "string", "description": "Conversation or source ID"}
                            },
                            "required": ["id", "content"]
                        }
                    },
                    "relations": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "from": {"type": "string"},
                                "to": {"type": "string"},
                                "type": {"type": "string"},
                                "weight": {"type": "number"}
                            },
                            "required": ["from", "to", "type"]
                        }
                    },
                    "check_contradictions": {"type": "boolean", "default": False, "description": "Check for contradicting facts before storing"}
                }
            }
        },
        {
            "name": "forget",
            "description": "Remove an entity and its relations",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "id": {"type": "string"}
                },
                "required": ["id"]
            }
        },
        {
            "name": "reflect",
            "description": "Consolidate knowledge: apply temporal decay, find similar entities, and identify clusters",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "apply_decay": {"type": "boolean", "default": True, "description": "Reduce weight of old unaccessed entities"},
                    "decay_days": {"type": "integer", "default": 30, "description": "Days after which decay starts"},
                    "find_similar": {"type": "boolean", "default": True, "description": "Find potentially duplicate entities"},
                    "similarity_threshold": {"type": "number", "default": 0.85, "description": "Threshold for considering entities similar"}
                }
            }
        },
        {
            "name": "check_contradictions",
            "description": "Find contradicting facts in the knowledge graph",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "entity_id": {"type": "string", "description": "Check contradictions for a specific entity"},
                    "labels": {"type": "array", "items": {"type": "string"}, "description": "Check within specific labels"}
                }
            }
        }
    ]


@server.call_tool()
async def call_tool(name: str, arguments: dict):
    if name == "search":
        query = arguments['query']
        query_embedding = embed(query)
        labels = arguments.get('labels', [])
        limit = arguments.get('limit', 10)
        semantic_weight = arguments.get('semantic_weight', 0.5)

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

        return {"content": [{"type": "text", "text": json.dumps(entities, indent=2, default=str)}]}

    elif name == "get_entity":
        entity_id = arguments['id']
        results = await db.query("""
            SELECT * FROM entity:$id
        """, {'id': entity_id})

        entity = results[0][0] if results and results[0] else None
        if entity:
            await db.query("""
                UPDATE entity:$id SET accessed = time::now(), access_count += 1
            """, {'id': entity_id})

        return {"content": [{"type": "text", "text": json.dumps(entity, indent=2, default=str)}]}

    elif name == "list_labels":
        results = await db.query("""
            SELECT array::distinct(array::flatten((SELECT labels FROM entity))) AS labels
        """)
        labels = results[0][0]['labels'] if results and results[0] else []
        return {"content": [{"type": "text", "text": json.dumps(labels, indent=2)}]}

    elif name == "traverse":
        start = arguments['start']
        depth = arguments.get('depth', 2)
        relation_types = arguments.get('relation_types', [])

        if relation_types:
            # Filtered traversal by relation type
            type_filter = '|'.join(relation_types)
            results = await db.query(f"""
                SELECT *, ->(({type_filter}))..{depth}->entity AS connected
                FROM entity:$id
            """, {'id': start})
        else:
            results = await db.query(f"""
                SELECT *, ->?..{depth}->entity AS connected
                FROM entity:$id
            """, {'id': start})

        return {"content": [{"type": "text", "text": json.dumps(results[0] if results else [], indent=2, default=str)}]}

    elif name == "find_path":
        from_id = arguments['from']
        to_id = arguments['to']
        max_depth = arguments.get('max_depth', 5)

        # Use SurrealDB 2.2+ native shortest path
        results = await db.query(f"""
            SELECT * FROM entity:$from.{{..{max_depth}+shortest=entity:$to}}->?->entity
        """, {'from': from_id, 'to': to_id})

        return {"content": [{"type": "text", "text": json.dumps(results[0] if results else [], indent=2, default=str)}]}

    elif name == "remember":
        stored = {"entities": 0, "relations": 0, "contradictions": []}
        check_for_contradictions = arguments.get('check_contradictions', False)

        for entity in arguments.get('entities', []):
            # Check for contradictions if requested
            if check_for_contradictions:
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
                UPSERT entity:$id SET
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

        for rel in arguments.get('relations', []):
            await db.query("""
                RELATE entity:$from->$type->entity:$to SET weight = $weight
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

        return {"content": [{"type": "text", "text": msg}]}

    elif name == "forget":
        entity_id = arguments['id']
        await db.query("""
            DELETE entity:$id;
            DELETE FROM entity:$id->?;
            DELETE FROM ?->entity:$id;
        """, {'id': entity_id})

        return {"content": [{"type": "text", "text": f"Removed {entity_id}"}]}

    elif name == "reflect":
        apply_decay = arguments.get('apply_decay', True)
        decay_days = arguments.get('decay_days', 30)
        find_similar = arguments.get('find_similar', True)
        similarity_threshold = arguments.get('similarity_threshold', 0.85)

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

        return {"content": [{"type": "text", "text": json.dumps(report, indent=2, default=str)}]}

    elif name == "check_contradictions":
        entity_id = arguments.get('entity_id')
        labels = arguments.get('labels', [])

        contradictions = []

        if entity_id:
            # Check specific entity against similar ones
            entity = await db.query("SELECT * FROM entity:$id", {'id': entity_id})
            entity = entity[0][0] if entity and entity[0] else None

            if entity:
                similar = await db.query("""
                    SELECT id, content FROM entity
                    WHERE embedding <|10,100|> $emb AND id != $id
                """, {'emb': entity['embedding'], 'id': f"entity:{entity_id}"})

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

        return {"content": [{"type": "text", "text": json.dumps(contradictions, indent=2)}]}

    return {"content": [{"type": "text", "text": f"Unknown tool: {name}"}]}


async def _run():
    await init()
    async with stdio_server() as (read, write):
        await server.run(read, write)


def main():
    import asyncio
    asyncio.run(_run())


if __name__ == "__main__":
    main()
