import asyncio
import json
import os
import sys
from datetime import datetime, timedelta
from typing import Any, cast
from pathlib import Path

# Early startup indicator
print("Starting memcp server (loading dependencies, this may take a moment)...", file=sys.stderr, flush=True)

from mcp.server.fastmcp import FastMCP, Context
from mcp import types
from sentence_transformers import SentenceTransformer, CrossEncoder

# Import from memcp modules
from memcp.models import (
    EntityResult, SearchResult, SimilarPair, Contradiction,
    ReflectResult, RememberResult, MemorizeFileResult, MemoryStats
)
from memcp.db import (
    app_lifespan, run_query,
    validate_entity, validate_relation, QueryResult,
    # Query functions
    query_hybrid_search, query_update_access, query_get_entity,
    query_list_labels, query_traverse, query_find_path,
    query_similar_entities, query_upsert_entity, query_create_relation,
    query_delete_entity, query_apply_decay, query_all_entities_with_embedding,
    query_similar_by_embedding, query_delete_entity_by_record_id,
    query_entity_with_embedding, query_similar_for_contradiction,
    query_entities_by_labels, query_vector_similarity,
    query_count_entities, query_count_relations,
    query_get_all_labels, query_count_by_label
)


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




# Create server with lifespan
mcp = FastMCP("knowledge-graph", lifespan=app_lifespan)


async def sample(ctx: Context, prompt: str, max_tokens: int = 1000) -> str:
    """Helper to call LLM via MCP sampling."""
    result = await ctx.request_context.session.create_message(
        messages=[
            types.SamplingMessage(
                role="user",
                content=types.TextContent(type="text", text=prompt)
            )
        ],
        max_tokens=max_tokens
    )
    return result.content.text


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
        # Extract entity ID from full record ID (e.g., "entity:user123" -> "user123")
        entity_id = str(r['id']).split(':', 1)[1] if ':' in str(r['id']) else str(r['id'])
        await run_query(ctx, """
            UPDATE type::thing("entity", $id) SET accessed = time::now(), access_count += 1
        """, {'id': entity_id})

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
        summary = await sample(
            ctx,
            f"Summarize these memory search results for the query '{query}' in 2-3 sentences. "
            f"Focus on the most relevant information.\n\nResults:\n{contents}"
        )

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
        SELECT array::distinct(array::flatten(array::group(labels))) AS labels FROM entity GROUP ALL
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
        SELECT * FROM type::thing("entity", $from)..{max_depth}->entity WHERE id = type::thing("entity", $to) LIMIT 1
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
            tag_response = await sample(
                ctx,
                f"Generate 3-5 short, lowercase category tags (comma-separated) for this content. "
                f"Only output the tags, nothing else.\n\nContent: {entity['content'][:500]}"
            )
            # Parse comma-separated tags
            tags = [t.strip().lower() for t in tag_response.split(',') if t.strip()]
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

    response = await sample(ctx, extraction_prompt, max_tokens=2000)

    # Parse the JSON response
    try:
        # Try to extract JSON from the response (handle potential markdown code blocks)
        response_text = response.strip()
        if response_text.startswith('```'):
            # Remove markdown code block
            lines = response_text.split('\n')
            response_text = '\n'.join(lines[1:-1] if lines[-1] == '```' else lines[1:])
        extracted = json.loads(response_text)
    except json.JSONDecodeError as e:
        await ctx.error(f"Failed to parse LLM response as JSON: {e}")
        raise ToolError(f"LLM did not return valid JSON. Response: {response[:200]}...")

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

                            # Extract entity ID from full record ID
                            delete_id = str(to_delete).split(':', 1)[1] if ':' in str(to_delete) else str(to_delete)
                            await run_query(ctx, """
                                DELETE type::thing("entity", $id);
                                DELETE FROM type::thing("entity", $id)->?;
                                DELETE FROM ?->type::thing("entity", $id);
                            """, {'id': delete_id})

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
                WHERE embedding <|10,100|> $emb AND id != $id
            """, {'emb': entity['embedding'], 'id': f"entity:{entity_id}"})

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

    # Count relations - count all outgoing edges from all entities
    relation_count = await run_query(ctx, """
        SELECT count() FROM (SELECT ->? FROM entity) GROUP ALL
    """)
    total_relations = relation_count[0][0]['count'] if relation_count and relation_count[0] else 0

    labels_result = await run_query(ctx, """
        SELECT array::distinct(array::flatten((SELECT labels FROM entity).labels)) AS labels
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
        SELECT array::distinct(array::flatten((SELECT labels FROM entity).labels)) AS labels
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
