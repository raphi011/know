import asyncio
import json
import os
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Any, cast

from pathlib import Path

# Early startup indicator
print("Starting memcp server (loading dependencies, this may take a moment)...", file=sys.stderr, flush=True)

from mcp.server.fastmcp import FastMCP, Context
from mcp.server.fastmcp.exceptions import ToolError
from pydantic import BaseModel, Field
from surrealdb import AsyncSurreal
from sentence_transformers import SentenceTransformer, CrossEncoder

# Type alias for query results - SurrealDB returns list[Value] but we know
# our queries return list of dicts in practice
QueryResult = list[list[dict[str, Any]]]

# Configuration from environment
SURREALDB_URL = os.getenv("SURREALDB_URL", "ws://localhost:8000/rpc")
SURREALDB_NAMESPACE = os.getenv("SURREALDB_NAMESPACE", "knowledge")
SURREALDB_DATABASE = os.getenv("SURREALDB_DATABASE", "graph")
SURREALDB_USER = os.getenv("SURREALDB_USER", "root")
SURREALDB_PASS = os.getenv("SURREALDB_PASS", "root")
SURREALDB_AUTH_LEVEL = os.getenv("SURREALDB_AUTH_LEVEL", "root")  # "root" or "database"
QUERY_TIMEOUT = float(os.getenv("MEMCP_QUERY_TIMEOUT", "30"))

# Embedding model: Transforms text into 384-dimensional vectors where semantically
# similar texts cluster together in vector space. "all-MiniLM-L6-v2" is a lightweight
# model trained on 1B+ sentence pairs - good balance of speed vs quality.
import sys
print("Loading embedding model (all-MiniLM-L6-v2)...", file=sys.stderr)
embedder = SentenceTransformer('all-MiniLM-L6-v2')
print("Embedding model loaded", file=sys.stderr)

# NLI (Natural Language Inference) model: Given two sentences, classifies their
# relationship as contradiction/entailment/neutral. Uses DeBERTa architecture
# fine-tuned on SNLI+MNLI datasets. CrossEncoder means both sentences are processed
# together (vs bi-encoder which encodes separately) - slower but more accurate.
print("Loading NLI model (cross-encoder/nli-deberta-v3-base)...", file=sys.stderr)
nli_model = CrossEncoder('cross-encoder/nli-deberta-v3-base')
print("NLI model loaded", file=sys.stderr)

# NLI output labels
NLI_LABELS = ['contradiction', 'entailment', 'neutral']


# =============================================================================
# Pydantic Response Models
# =============================================================================

class EntityResult(BaseModel):
    """A memory entity returned from search or retrieval."""
    id: str
    type: str | None = None
    labels: list[str] = Field(default_factory=list)
    content: str
    confidence: float | None = None
    source: str | None = None
    decay_weight: float | None = None


class SearchResult(BaseModel):
    """Result from a memory search."""
    entities: list[EntityResult] = Field(default_factory=list)
    count: int = 0
    summary: str | None = None


class SimilarPair(BaseModel):
    """A pair of similar entities found during reflection."""
    entity1: EntityResult
    entity2: EntityResult
    similarity: float


class Contradiction(BaseModel):
    """A contradiction detected between two entities."""
    entity1: EntityResult
    entity2: EntityResult
    confidence: float


class ReflectResult(BaseModel):
    """Result from the reflect maintenance operation."""
    decayed: int = 0
    similar_pairs: list[SimilarPair] = Field(default_factory=list)
    merged: int = 0


class RememberResult(BaseModel):
    """Result from storing memories."""
    entities_stored: int = 0
    relations_stored: int = 0
    contradictions: list[Contradiction] = Field(default_factory=list)


class MemorizeFileResult(BaseModel):
    """Result from memorizing a file."""
    file_path: str
    entities_stored: int = 0
    relations_stored: int = 0
    content_length: int = 0


class MemoryStats(BaseModel):
    """Statistics about the memory store."""
    total_entities: int = 0
    total_relations: int = 0
    labels: list[str] = Field(default_factory=list)
    label_counts: dict[str, int] = Field(default_factory=dict)


# =============================================================================
# Lifespan Context & Database Management
# =============================================================================

@dataclass
class AppContext:
    """Application context available during server lifetime."""
    db: AsyncSurreal
    initialized: bool = False


@asynccontextmanager
async def app_lifespan(server: FastMCP) -> AsyncIterator[AppContext]:
    """Manage database connection lifecycle."""
    import sys
    db = AsyncSurreal(SURREALDB_URL)
    ctx = AppContext(db=db)

    try:
        # Connect to database
        print(f"Connecting to SurrealDB at {SURREALDB_URL}...", file=sys.stderr)
        async with asyncio.timeout(QUERY_TIMEOUT):
            await db.connect()
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
        await db.query(cast(Any, """
            DEFINE TABLE IF NOT EXISTS entity SCHEMAFULL;
            DEFINE FIELD IF NOT EXISTS type ON entity TYPE string;
            DEFINE FIELD IF NOT EXISTS labels ON entity TYPE array<string>;
            DEFINE FIELD IF NOT EXISTS content ON entity TYPE string;
            DEFINE FIELD IF NOT EXISTS embedding ON entity TYPE array<float>;
            DEFINE FIELD IF NOT EXISTS confidence ON entity TYPE float DEFAULT 1.0;
            DEFINE FIELD IF NOT EXISTS source ON entity TYPE string;
            DEFINE FIELD IF NOT EXISTS decay_weight ON entity TYPE float DEFAULT 1.0;
            DEFINE FIELD IF NOT EXISTS created ON entity TYPE datetime DEFAULT time::now();
            DEFINE FIELD IF NOT EXISTS accessed ON entity TYPE datetime DEFAULT time::now();
            DEFINE FIELD IF NOT EXISTS access_count ON entity TYPE int DEFAULT 0;

            DEFINE INDEX IF NOT EXISTS entity_labels ON entity FIELDS labels;
            DEFINE INDEX IF NOT EXISTS entity_embedding ON entity FIELDS embedding MTREE DIMENSION 384 DIST COSINE;
            DEFINE ANALYZER IF NOT EXISTS entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english);
            DEFINE INDEX IF NOT EXISTS entity_content_ft ON entity FIELDS content SEARCH ANALYZER entity_analyzer BM25;
        """))
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


# Create server with lifespan
mcp = FastMCP("knowledge-graph", lifespan=app_lifespan)


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


def get_db(ctx: Context) -> AsyncSurreal:
    """Get database connection from context."""
    app_ctx: AppContext = ctx.request_context.lifespan_context
    return app_ctx.db


async def run_query(ctx: Context, sql: str, vars: dict[str, Any] | None = None) -> QueryResult:
    """Execute a database query with timeout and error handling."""
    db = get_db(ctx)
    try:
        async with asyncio.timeout(QUERY_TIMEOUT):
            result = await db.query(sql, cast(Any, vars))
            return cast(QueryResult, result)
    except asyncio.TimeoutError:
        await ctx.error(f"Query timed out after {QUERY_TIMEOUT}s")
        raise ToolError(f"Database query timed out after {QUERY_TIMEOUT}s")
    except ToolError:
        raise
    except Exception as e:
        await ctx.error(f"Database query failed: {e}")
        raise ToolError(f"Database query failed: {e}")


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


@mcp.tool(annotations={"readOnlyHint": True})
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    semantic_weight: float = 0.5,
    summarize: bool = False,
    ctx: Context = None  # type: ignore[assignment]
) -> SearchResult:
    """Search your persistent memory for previously stored knowledge. Use when the user asks 'do you remember...', 'what do you know about...', 'recall...', or needs context from past conversations. Combines semantic similarity and keyword matching."""
    if not query or not query.strip():
        raise ToolError("Query cannot be empty")
    if limit < 1 or limit > 100:
        raise ToolError("Limit must be between 1 and 100")
    if not 0 <= semantic_weight <= 1:
        raise ToolError("semantic_weight must be between 0 and 1")

    await ctx.info(f"Searching for: {query[:50]}...")

    query_embedding = embed(query)
    filter_labels = labels or []
    label_filter = "AND labels CONTAINSANY $labels" if filter_labels else ""

    # Hybrid search: BM25 keyword matching + vector similarity
    results = await run_query(ctx, f"""
        SELECT id, type, labels, content, confidence, source, decay_weight,
               search::score(1) AS bm25_score,
               vector::similarity::cosine(embedding, $emb) AS vec_score
        FROM entity
        WHERE (content @1@ $q OR embedding <|{limit * 2},100|> $emb) {label_filter}
        ORDER BY (vec_score * $sem_weight + search::score(1) * (1 - $sem_weight)) DESC
        LIMIT $limit
    """, {
        'q': query,
        'emb': query_embedding,
        'labels': filter_labels,
        'limit': limit,
        'sem_weight': semantic_weight
    })

    # Track access patterns
    entities = results[0] if results and results[0] else []
    for r in entities:
        await run_query(ctx, """
            UPDATE $id SET accessed = time::now(), access_count += 1
        """, {'id': r['id']})

    await ctx.info(f"Found {len(entities)} results")

    entity_results = [EntityResult(
        id=str(e['id']),
        type=e.get('type'),
        labels=e.get('labels', []),
        content=e['content'],
        confidence=e.get('confidence'),
        source=e.get('source'),
        decay_weight=e.get('decay_weight')
    ) for e in entities]

    # Generate summary using LLM if requested
    summary = None
    if summarize and entity_results:
        await ctx.info("Generating summary of search results")
        contents = "\n---\n".join([f"[{e.id}]: {e.content[:200]}" for e in entity_results[:10]])
        summary_response = await ctx.sample(
            f"Summarize these memory search results for the query '{query}' in 2-3 sentences. "
            f"Focus on the most relevant information.\n\nResults:\n{contents}"
        )
        summary = summary_response.text

    return SearchResult(entities=entity_results, count=len(entity_results), summary=summary)


@mcp.tool(annotations={"readOnlyHint": True})
async def get_entity(entity_id: str, ctx: Context) -> EntityResult | None:
    """Retrieve a specific memory by its ID. Use when you have an exact entity ID from a previous search or traversal."""
    if not entity_id or not entity_id.strip():
        raise ToolError("ID cannot be empty")

    results = await run_query(ctx, """
        SELECT * FROM type::thing("entity", $id)
    """, {'id': entity_id})

    entity = results[0][0] if results and results[0] else None
    if entity:
        await run_query(ctx, """
            UPDATE type::thing("entity", $id) SET accessed = time::now(), access_count += 1
        """, {'id': entity_id})

        return EntityResult(
            id=str(entity['id']),
            type=entity.get('type'),
            labels=entity.get('labels', []),
            content=entity['content'],
            confidence=entity.get('confidence'),
            source=entity.get('source'),
            decay_weight=entity.get('decay_weight')
        )
    return None


@mcp.tool(annotations={"readOnlyHint": True, "idempotentHint": True})
async def list_labels(ctx: Context) -> list[str]:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    results = await run_query(ctx, """
        SELECT array::distinct(array::flatten((SELECT labels FROM entity))) AS labels
    """)
    return results[0][0]['labels'] if results and results[0] else []


@mcp.tool(annotations={"readOnlyHint": True, "idempotentHint": True})
async def traverse(
    start: str,
    depth: int = 2,
    relation_types: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> str:
    """Explore how stored knowledge connects to other knowledge. Use when the user asks 'what's related to...', 'how does X connect to Y', or wants to understand context around a topic."""
    if not start or not start.strip():
        raise ToolError("start ID cannot be empty")
    if depth < 1 or depth > 10:
        raise ToolError("depth must be between 1 and 10")

    await ctx.info(f"Traversing from {start} with depth {depth}")

    relation_types = relation_types or []

    if relation_types:
        type_filter = '|'.join(relation_types)
        results = await run_query(ctx, f"""
            SELECT *, ->(({type_filter}))..{depth}->entity AS connected
            FROM type::thing("entity", $id)
        """, {'id': start})
    else:
        results = await run_query(ctx, f"""
            SELECT *, ->?..{depth}->entity AS connected
            FROM type::thing("entity", $id)
        """, {'id': start})

    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool(annotations={"readOnlyHint": True, "idempotentHint": True})
async def find_path(
    from_id: str,
    to_id: str,
    max_depth: int = 5,
    ctx: Context = None  # type: ignore[assignment]
) -> str:
    """Find how two pieces of knowledge are connected through intermediate relationships. Use when the user asks 'how is X related to Y' or wants to trace connections between concepts."""
    if not from_id or not from_id.strip():
        raise ToolError("from_id cannot be empty")
    if not to_id or not to_id.strip():
        raise ToolError("to_id cannot be empty")
    if max_depth < 1 or max_depth > 20:
        raise ToolError("max_depth must be between 1 and 20")

    await ctx.info(f"Finding path from {from_id} to {to_id}")

    results = await run_query(ctx, f"""
        SELECT * FROM type::thing("entity", $from).{{..{max_depth}+shortest=type::thing("entity", $to)}}->?->entity
    """, {'from': from_id, 'to': to_id})

    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool(annotations={"destructiveHint": False})
async def remember(
    entities: list[dict] | None = None,
    relations: list[dict] | None = None,
    detect_contradictions: bool = False,
    auto_tag: bool = False,
    ctx: Context = None  # type: ignore[assignment]
) -> RememberResult:
    """Store important information in persistent memory for future sessions. Use proactively when the user shares preferences, facts about themselves, project context, decisions, or anything they'd want you to recall later. Supports confidence scores and contradiction detection.

    entities: list of {id, content, type?, labels?, confidence?, source?}
    relations: list of {from, to, type, weight?}
    auto_tag: if True, uses LLM to generate labels for entities without labels
    """
    entities = entities or []
    relations = relations or []

    # Validate all inputs before any DB operations
    for i, entity in enumerate(entities):
        validate_entity(entity)

    for i, relation in enumerate(relations):
        validate_relation(relation)

    result = RememberResult()

    await ctx.info(f"Storing {len(entities)} entities and {len(relations)} relations")

    for entity in entities:
        # Auto-generate labels using LLM if requested and no labels provided
        if auto_tag and not entity.get('labels'):
            await ctx.info(f"Auto-tagging entity: {entity['id']}")
            tag_response = await ctx.sample(
                f"Generate 3-5 short, lowercase category tags (comma-separated) for this content. "
                f"Only output the tags, nothing else.\n\nContent: {entity['content'][:500]}"
            )
            # Parse comma-separated tags
            tags = [t.strip().lower() for t in tag_response.text.split(',') if t.strip()]
            entity['labels'] = tags[:5]  # Limit to 5 tags

        if detect_contradictions:
            similar = await run_query(ctx, """
                SELECT id, content FROM entity
                WHERE embedding <|5,100|> $emb AND id != $id
            """, {
                'emb': embed(entity['content']),
                'id': f"entity:{entity['id']}"
            })

            for existing in (similar[0] if similar else []):
                nli_result = check_contradiction(entity['content'], existing['content'])
                if nli_result['label'] == 'contradiction':
                    result.contradictions.append(Contradiction(
                        entity1=EntityResult(id=entity['id'], content=entity['content']),
                        entity2=EntityResult(id=str(existing['id']), content=existing['content']),
                        confidence=nli_result['scores']['contradiction']
                    ))

        await run_query(ctx, """
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
        result.entities_stored += 1

    for rel in relations:
        await run_query(ctx, """
            RELATE type::thing("entity", $from)->$type->type::thing("entity", $to) SET weight = $weight
        """, {
            'from': rel['from'],
            'type': rel['type'],
            'to': rel['to'],
            'weight': rel.get('weight', 1.0)
        })
        result.relations_stored += 1

    await ctx.info(f"Stored {result.entities_stored} entities, {result.relations_stored} relations")

    return result


def _read_file_content(file_path: str) -> str:
    """Read file content based on file type."""
    path = Path(file_path)
    if not path.exists():
        raise ToolError(f"File not found: {file_path}")

    suffix = path.suffix.lower()

    if suffix == '.pdf':
        from pypdf import PdfReader
        reader = PdfReader(file_path)
        pages = [page.extract_text() for page in reader.pages]
        return "\n\n".join(pages)
    elif suffix in ('.md', '.markdown', '.txt', '.text', '.rst'):
        return path.read_text(encoding='utf-8')
    else:
        # Try to read as text
        try:
            return path.read_text(encoding='utf-8')
        except UnicodeDecodeError:
            raise ToolError(f"Cannot read file as text: {file_path}. Supported formats: .pdf, .md, .txt")


@mcp.tool()
async def memorize_file(
    file_path: str,
    source: str | None = None,
    labels: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> MemorizeFileResult:
    """Read a file (PDF, markdown, text) and extract key information to store in memory.

    Uses LLM to intelligently extract the most important facts, concepts, and relationships
    from the document and stores them as searchable memory entities.

    Args:
        file_path: Path to the file to memorize
        source: Optional source identifier (defaults to filename)
        labels: Optional labels to apply to all extracted entities
    """
    await ctx.info(f"Reading file: {file_path}")

    # Read file content
    content = _read_file_content(file_path)
    content_length = len(content)

    if not content.strip():
        raise ToolError("File is empty")

    await ctx.info(f"Read {content_length} characters, extracting knowledge via LLM...")

    # Use LLM to extract structured knowledge
    extraction_prompt = f"""Analyze this document and extract the key information worth remembering.

Return a JSON object with this exact structure:
{{
  "entities": [
    {{
      "id": "unique-kebab-case-id",
      "content": "A clear, self-contained statement of the fact or concept",
      "type": "fact|concept|procedure|preference|definition",
      "labels": ["relevant", "category", "tags"]
    }}
  ],
  "relations": [
    {{
      "from": "entity-id-1",
      "to": "entity-id-2",
      "type": "relates_to|contains|requires|contradicts|supports"
    }}
  ]
}}

Guidelines:
- Extract 5-20 of the most important, standalone facts or concepts
- Each entity content should be self-contained and understandable without context
- Use descriptive, unique IDs based on the content
- Include relations between entities where meaningful connections exist
- Focus on information that would be useful to recall later

Document content:
{content}

Return ONLY the JSON object, no other text."""

    response = await ctx.sample(extraction_prompt)

    # Parse the JSON response
    try:
        # Try to extract JSON from the response (handle potential markdown code blocks)
        response_text = response.text.strip()
        if response_text.startswith('```'):
            # Remove markdown code block
            lines = response_text.split('\n')
            response_text = '\n'.join(lines[1:-1] if lines[-1] == '```' else lines[1:])
        extracted = json.loads(response_text)
    except json.JSONDecodeError as e:
        await ctx.error(f"Failed to parse LLM response as JSON: {e}")
        raise ToolError(f"LLM did not return valid JSON. Response: {response.text[:200]}...")

    # Apply source and additional labels
    file_source = source or Path(file_path).name
    additional_labels = labels or []

    entities = extracted.get('entities', [])
    relations = extracted.get('relations', [])

    # Add source and merge labels
    for entity in entities:
        entity['source'] = file_source
        existing_labels = entity.get('labels', [])
        entity['labels'] = list(set(existing_labels + additional_labels))

    await ctx.info(f"Extracted {len(entities)} entities and {len(relations)} relations")

    # Store using the existing remember infrastructure
    result = await remember(
        entities=entities,
        relations=relations,
        detect_contradictions=False,
        auto_tag=False,
        ctx=ctx
    )

    return MemorizeFileResult(
        file_path=file_path,
        entities_stored=result.entities_stored,
        relations_stored=result.relations_stored,
        content_length=content_length
    )


@mcp.tool(annotations={"destructiveHint": True})
async def forget(entity_id: str, ctx: Context) -> str:
    """Delete information from persistent memory. Use when the user says 'forget this', 'remove...', 'delete...', or when information is explicitly outdated or wrong."""
    if not entity_id or not entity_id.strip():
        raise ToolError("ID cannot be empty")

    await ctx.info(f"Deleting entity: {entity_id}")

    await run_query(ctx, """
        DELETE type::thing("entity", $id);
        DELETE FROM type::thing("entity", $id)->?;
        DELETE FROM ?->type::thing("entity", $id);
    """, {'id': entity_id})

    return f"Removed {entity_id}"


@mcp.tool(annotations={"destructiveHint": True})
async def reflect(
    apply_decay: bool = True,
    decay_days: int = 30,
    find_similar: bool = True,
    similarity_threshold: float = 0.85,
    auto_merge: bool = False,
    ctx: Context = None  # type: ignore[assignment]
) -> ReflectResult:
    """Maintenance tool to clean up memory: decay old unused knowledge, find duplicates, identify clusters. Use periodically or when the user asks to 'clean up', 'organize', or 'consolidate' memories.

    When auto_merge is True, duplicate entities (similarity >= threshold) are automatically
    merged: the entity with higher access_count is kept, the other is deleted. If access
    counts are equal, the more recently accessed entity is kept.
    """
    if decay_days < 1:
        raise ToolError("decay_days must be at least 1")
    if not 0 <= similarity_threshold <= 1:
        raise ToolError("similarity_threshold must be between 0 and 1")

    # Read current stats via resource to log context
    stats = await ctx.read_resource("memory://stats")
    await ctx.info(f"Starting reflect on {stats} entities")

    result = ReflectResult()

    if apply_decay:
        await ctx.info("Applying temporal decay to old memories")
        cutoff = datetime.now() - timedelta(days=decay_days)
        decay_result = await run_query(ctx, """
            UPDATE entity SET decay_weight = decay_weight * 0.9
            WHERE accessed < $cutoff AND decay_weight > 0.1
        """, {'cutoff': cutoff.isoformat()})
        result.decayed = len(decay_result[0]) if decay_result and decay_result[0] else 0

    if find_similar:
        await ctx.info("Finding similar entities")
        entities = await run_query(ctx, "SELECT id, content, embedding, access_count, accessed FROM entity")
        entities = entities[0] if entities else []

        seen_pairs: set[tuple[str, str]] = set()
        deleted_ids: set[str] = set()

        for entity in entities:
            if str(entity['id']) in deleted_ids:
                continue

            similar = await run_query(ctx, """
                SELECT id, content, access_count, accessed,
                       vector::similarity::cosine(embedding, $emb) AS sim
                FROM entity
                WHERE embedding <|10,100|> $emb AND id != $id
            """, {'emb': entity['embedding'], 'id': entity['id']})

            for candidate in (similar[0] if similar else []):
                if str(candidate['id']) in deleted_ids:
                    continue

                if candidate['sim'] >= similarity_threshold:
                    id1, id2 = str(entity['id']), str(candidate['id'])
                    pair_key = (id1, id2) if id1 < id2 else (id2, id1)
                    if pair_key not in seen_pairs:
                        seen_pairs.add(pair_key)

                        if auto_merge:
                            e_count = entity.get('access_count', 0)
                            c_count = candidate.get('access_count', 0)
                            e_accessed = entity.get('accessed', '')
                            c_accessed = candidate.get('accessed', '')

                            if c_count > e_count or (c_count == e_count and c_accessed > e_accessed):
                                to_delete = entity['id']
                            else:
                                to_delete = candidate['id']

                            await run_query(ctx, """
                                DELETE $id;
                                DELETE FROM $id->?;
                                DELETE FROM ?->$id;
                            """, {'id': to_delete})

                            deleted_ids.add(str(to_delete))
                            result.merged += 1
                            await ctx.info(f"Merged duplicate: deleted {to_delete}")
                        else:
                            result.similar_pairs.append(SimilarPair(
                                entity1=EntityResult(id=str(entity['id']), content=entity['content'][:100]),
                                entity2=EntityResult(id=str(candidate['id']), content=candidate['content'][:100]),
                                similarity=candidate['sim']
                            ))

    await ctx.info(f"Reflect complete: {result.decayed} decayed, {len(result.similar_pairs)} similar, {result.merged} merged")
    return result


@mcp.tool(annotations={"readOnlyHint": True, "idempotentHint": True})
async def check_contradictions_tool(
    entity_id: str | None = None,
    labels: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> list[Contradiction]:
    """Detect conflicting information in memory. Use when the user asks to verify consistency, or proactively before storing facts that might conflict with existing knowledge."""
    labels = labels or []
    contradictions: list[Contradiction] = []

    await ctx.info("Checking for contradictions")

    if entity_id:
        entity = await run_query(ctx, 'SELECT * FROM type::thing("entity", $id)', {'id': entity_id})
        entity = entity[0][0] if entity and entity[0] else None

        if entity:
            similar = await run_query(ctx, """
                SELECT id, content FROM entity
                WHERE embedding <|10,100|> $emb AND id != type::thing("entity", $id)
            """, {'emb': entity['embedding'], 'id': entity_id})

            for other in (similar[0] if similar else []):
                nli_result = check_contradiction(entity['content'], other['content'])
                if nli_result['label'] == 'contradiction':
                    contradictions.append(Contradiction(
                        entity1=EntityResult(id=entity_id, content=entity['content']),
                        entity2=EntityResult(id=str(other['id']), content=other['content']),
                        confidence=nli_result['scores']['contradiction']
                    ))
    else:
        label_filter = "WHERE labels CONTAINSANY $labels" if labels else ""
        entities = await run_query(ctx, f"SELECT id, content, embedding FROM entity {label_filter}", {'labels': labels})
        entities = entities[0] if entities else []

        for i, e1 in enumerate(entities):
            for e2 in entities[i+1:]:
                similar = await run_query(ctx, """
                    SELECT vector::similarity::cosine($emb1, $emb2) AS sim
                """, {'emb1': e1['embedding'], 'emb2': e2['embedding']})

                sim = similar[0][0]['sim'] if similar and similar[0] else 0
                if sim > 0.5:
                    nli_result = check_contradiction(e1['content'], e2['content'])
                    if nli_result['label'] == 'contradiction':
                        contradictions.append(Contradiction(
                            entity1=EntityResult(id=str(e1['id']), content=e1['content']),
                            entity2=EntityResult(id=str(e2['id']), content=e2['content']),
                            confidence=nli_result['scores']['contradiction']
                        ))

    await ctx.info(f"Found {len(contradictions)} contradictions")
    return contradictions


# =============================================================================
# Resources - Read-only data endpoints
# =============================================================================

@mcp.resource("memory://stats")
async def get_memory_stats(ctx: Context) -> MemoryStats:
    """Get statistics about the memory store."""
    entity_count = await run_query(ctx, "SELECT count() FROM entity GROUP ALL")
    total_entities = entity_count[0][0]['count'] if entity_count and entity_count[0] else 0

    # Count relations by querying all edge tables
    # Note: This is approximate as we'd need to know all relation types
    relation_count = await run_query(ctx, """
        SELECT count() FROM (SELECT * FROM ->?) GROUP ALL
    """)
    total_relations = relation_count[0][0]['count'] if relation_count and relation_count[0] else 0

    labels_result = await run_query(ctx, """
        SELECT array::distinct(array::flatten((SELECT labels FROM entity))) AS labels
    """)
    all_labels = labels_result[0][0]['labels'] if labels_result and labels_result[0] else []

    # Count entities per label
    label_counts: dict[str, int] = {}
    for label in all_labels:
        count_result = await run_query(ctx, """
            SELECT count() FROM entity WHERE labels CONTAINS $label GROUP ALL
        """, {'label': label})
        label_counts[label] = count_result[0][0]['count'] if count_result and count_result[0] else 0

    return MemoryStats(
        total_entities=total_entities,
        total_relations=total_relations,
        labels=all_labels,
        label_counts=label_counts
    )


@mcp.resource("memory://labels")
async def get_all_labels(ctx: Context) -> list[str]:
    """Get all labels/categories used in memory."""
    results = await run_query(ctx, """
        SELECT array::distinct(array::flatten((SELECT labels FROM entity))) AS labels
    """)
    return results[0][0]['labels'] if results and results[0] else []


# =============================================================================
# Prompts - Reusable prompt templates
# =============================================================================

@mcp.prompt()
def summarize_memories(topic: str) -> str:
    """Generate a prompt to summarize what's known about a topic."""
    return f"""Please search my memory for everything related to "{topic}" and provide a comprehensive summary.

Include:
- Key facts and information stored about this topic
- Any related concepts or connections
- Confidence levels if available
- When information was last accessed

Use the search tool with the query "{topic}" to find relevant memories, then synthesize the results into a clear summary."""


@mcp.prompt()
def recall_context(task: str) -> str:
    """Generate a prompt to recall relevant context for a task."""
    return f"""I need to work on the following task: {task}

Please search my memory for any relevant context that might help with this task. Look for:
- Previous decisions or preferences related to this
- Technical details or configurations
- Past experiences or lessons learned
- Related projects or concepts

Use the search tool to find relevant memories and present the most useful context for completing this task."""


@mcp.prompt()
def find_connections(concept1: str, concept2: str) -> str:
    """Generate a prompt to explore connections between two concepts."""
    return f"""I want to understand how "{concept1}" and "{concept2}" are connected in my memory.

Please:
1. Search for memories about each concept
2. Use traverse to explore their connections
3. Use find_path to see if there's a direct relationship
4. Summarize how these concepts relate to each other based on stored knowledge"""


@mcp.prompt()
def memory_health_check() -> str:
    """Generate a prompt to check memory health and suggest cleanup."""
    return """Please perform a health check on my memory store:

1. Check the memory stats resource to see overall statistics
2. Use reflect(find_similar=true) to identify potential duplicates
3. Use check_contradictions_tool to find conflicting information
4. Provide recommendations for cleanup or consolidation

Do NOT auto-merge or delete anything - just report findings and suggestions."""


def main():
    try:
        mcp.run(transport="stdio")
    except KeyboardInterrupt:
        # Let Python handle it silently
        pass
    except Exception:
        # Exceptions during startup are already logged in app_lifespan
        # Just exit cleanly without printing the full stack trace
        sys.exit(1)


if __name__ == "__main__":
    main()
