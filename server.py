from mcp.server import Server
from mcp.server.stdio import stdio_server
from surrealdb import Surreal
from sentence_transformers import SentenceTransformer

server = Server("knowledge-graph")
db = Surreal()
embedder = SentenceTransformer('all-MiniLM-L6-v2')


async def init():
    await db.connect("ws://localhost:8000/rpc")
    await db.use("knowledge", "graph")

    # Schema setup
    await db.query("""
        DEFINE TABLE entity SCHEMAFULL;
        DEFINE FIELD type ON entity TYPE string;
        DEFINE FIELD labels ON entity TYPE array<string>;
        DEFINE FIELD content ON entity TYPE string;
        DEFINE FIELD embedding ON entity TYPE array<float>;
        DEFINE FIELD created ON entity TYPE datetime DEFAULT time::now();
        DEFINE FIELD accessed ON entity TYPE datetime DEFAULT time::now();
        DEFINE FIELD access_count ON entity TYPE int DEFAULT 0;

        DEFINE INDEX entity_labels ON entity FIELDS labels;
        DEFINE INDEX entity_embedding ON entity FIELDS embedding MTREE DIMENSION 384 DIST COSINE;
    """)


def embed(text: str) -> list[float]:
    return embedder.encode(text).tolist()


@server.list_tools()
async def list_tools():
    return [
        {
            "name": "search",
            "description": "Semantic search over knowledge graph",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                    "labels": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["query"]
            }
        },
        {
            "name": "traverse",
            "description": "Explore connections from an entity",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "start": {"type": "string"},
                    "depth": {"type": "integer", "default": 2},
                    "relation_types": {"type": "array", "items": {"type": "string"}}
                },
                "required": ["start"]
            }
        },
        {
            "name": "remember",
            "description": "Store entities and relations",
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
                                "content": {"type": "string"}
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
                    }
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
        }
    ]


@server.call_tool()
async def call_tool(name: str, arguments: dict):
    if name == "search":
        query_embedding = embed(arguments['query'])
        labels = arguments.get('labels', [])
        label_filter = "AND labels CONTAINSANY $labels" if labels else ""

        results = await db.query(f"""
            SELECT id, type, labels, content,
                   vector::similarity::cosine(embedding, $emb) AS score
            FROM entity
            WHERE embedding <|10,100|> $emb {label_filter}
            ORDER BY score DESC
        """, {'emb': query_embedding, 'labels': labels})

        # Update access metadata
        for r in results[0] if results else []:
            await db.query("""
                UPDATE $id SET accessed = time::now(), access_count += 1
            """, {'id': r['id']})

        return {"content": [{"type": "text", "text": str(results)}]}

    elif name == "traverse":
        depth = arguments.get('depth', 2)

        results = await db.query(f"""
            SELECT *, ->?..{depth}->entity AS connected
            FROM entity:$id
        """, {'id': arguments['start']})

        return {"content": [{"type": "text", "text": str(results)}]}

    elif name == "remember":
        stored = {"entities": 0, "relations": 0}

        for entity in arguments.get('entities', []):
            await db.query("""
                UPSERT entity:$id SET
                    type = $type,
                    labels = $labels,
                    content = $content,
                    embedding = $embedding
            """, {
                'id': entity['id'],
                'type': entity.get('type', 'concept'),
                'labels': entity.get('labels', []),
                'content': entity['content'],
                'embedding': embed(entity['content'])
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

        return {"content": [{"type": "text", "text": f"Stored {stored['entities']} entities, {stored['relations']} relations"}]}

    elif name == "forget":
        await db.query("""
            DELETE entity:$id;
            DELETE FROM $id->?;
            DELETE FROM ?->$id;
        """, {'id': arguments['id']})

        return {"content": [{"type": "text", "text": f"Removed {arguments['id']}"}]}


async def main():
    await init()
    async with stdio_server() as (read, write):
        await server.run(read, write)


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
