import asyncio
import json
import logging
import os
import sys
from datetime import datetime, timedelta
from typing import Any, cast

# Setup file logging
LOG_FILE = os.getenv("MEMCP_LOG_FILE", "/tmp/memcp.log")
LOG_LEVEL = os.getenv("MEMCP_LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format='%(asctime)s [%(levelname)s] %(message)s',
    handlers=[
        logging.FileHandler(LOG_FILE),
        logging.StreamHandler(sys.stderr)
    ]
)
logger = logging.getLogger("memcp")
logger.info(f"memcp logging to {LOG_FILE} (level: {LOG_LEVEL})")

# Early startup indicator
print("Starting memcp server (loading dependencies, this may take a moment)...", file=sys.stderr, flush=True)

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations
from sentence_transformers import SentenceTransformer, CrossEncoder

# Import from memcp modules
from memcp.models import (
    EntityResult, SearchResult, SimilarPair, Contradiction,
    ReflectResult, RememberResult, MemoryStats
)
from memcp.db import (
    app_lifespan, validate_entity, validate_relation,
    # Query functions
    query_hybrid_search, query_update_access, query_get_entity,
    query_list_labels, query_traverse, query_find_path,
    query_upsert_entity, query_create_relation,
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


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    semantic_weight: float = 0.5,
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

    # Hybrid search: BM25 keyword matching + vector similarity
    entities = await query_hybrid_search(
        ctx, query, query_embedding, filter_labels, limit, semantic_weight
    )

    # Track access patterns
    for r in entities:
        # Extract entity ID from full record ID (e.g., "entity:user123" -> "user123")
        entity_id = str(r['id']).split(':', 1)[1] if ':' in str(r['id']) else str(r['id'])
        await query_update_access(ctx, entity_id)

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

    return SearchResult(entities=entity_results, count=len(entity_results), summary=None)


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def get_entity(entity_id: str, ctx: Context) -> EntityResult | None:
    """Retrieve a specific memory by its ID. Use when you have an exact entity ID from a previous search or traversal."""
    if not entity_id or not entity_id.strip():
        raise ToolError("ID cannot be empty")

    results = await query_get_entity(ctx, entity_id)
    entity = results[0] if results else None

    if entity:
        await query_update_access(ctx, entity_id)

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


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def list_labels(ctx: Context) -> list[str]:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    results = await query_list_labels(ctx)
    return results[0]['labels'] if results else []


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
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

    results = await query_traverse(ctx, start, depth, relation_types or None)
    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
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

    results = await query_find_path(ctx, from_id, to_id, max_depth)
    return json.dumps(results[0] if results else [], indent=2, default=str)


@mcp.tool(annotations=ToolAnnotations(destructiveHint=False))
async def remember(
    entities: list[dict] | None = None,
    relations: list[dict] | None = None,
    detect_contradictions: bool = False,
    ctx: Context = None  # type: ignore[assignment]
) -> RememberResult:
    """Store important information in persistent memory for future sessions. Use proactively when the user shares preferences, facts about themselves, project context, decisions, or anything they'd want you to recall later. Supports confidence scores and contradiction detection.

    entities: list of {id, content, type?, labels?, confidence?, source?}
    relations: list of {from, to, type, weight?}
    """
    entities = entities or []
    relations = relations or []

    # Validate all inputs before any DB operations
    for i, entity in enumerate(entities):
        validate_entity(entity)

    for i, relation in enumerate(relations):
        validate_relation(relation)

    result = RememberResult()

    logger.info(f"remember() called with {len(entities)} entities, {len(relations)} relations")
    await ctx.info(f"Storing {len(entities)} entities and {len(relations)} relations")

    for entity in entities:
        if detect_contradictions:
            entity_embedding = embed(entity['content'])
            similar = await query_similar_for_contradiction(ctx, entity_embedding, entity['id'])

            for existing in similar:
                nli_result = check_contradiction(entity['content'], existing['content'])
                if nli_result['label'] == 'contradiction':
                    result.contradictions.append(Contradiction(
                        entity1=EntityResult(id=entity['id'], content=entity['content']),
                        entity2=EntityResult(id=str(existing['id']), content=existing['content']),
                        confidence=nli_result['scores']['contradiction']
                    ))

        logger.info(f"Upserting entity: {entity['id']}")
        try:
            embedding = embed(entity['content'])
            logger.debug(f"Generated embedding of length {len(embedding)}")
            upsert_result = await query_upsert_entity(
                ctx,
                entity_id=entity['id'],
                entity_type=entity.get('type', 'concept'),
                labels=entity.get('labels', []),
                content=entity['content'],
                embedding=embedding,
                confidence=entity.get('confidence', 1.0),
                source=entity.get('source')
            )
            logger.info(f"Upsert result for {entity['id']}: {upsert_result}")
            result.entities_stored += 1
        except Exception as e:
            logger.error(f"Failed to upsert entity {entity['id']}: {e}", exc_info=True)
            raise

    for rel in relations:
        await query_create_relation(
            ctx,
            from_id=rel['from'],
            rel_type=rel['type'],
            to_id=rel['to'],
            weight=rel.get('weight', 1.0)
        )
        result.relations_stored += 1

    await ctx.info(f"Stored {result.entities_stored} entities, {result.relations_stored} relations")

    return result


@mcp.tool(annotations=ToolAnnotations(destructiveHint=True))
async def forget(entity_id: str, ctx: Context) -> str:
    """Delete information from persistent memory. Use when the user says 'forget this', 'remove...', 'delete...', or when information is explicitly outdated or wrong."""
    if not entity_id or not entity_id.strip():
        raise ToolError("ID cannot be empty")

    await ctx.info(f"Deleting entity: {entity_id}")
    await query_delete_entity(ctx, entity_id)
    return f"Removed {entity_id}"


@mcp.tool(annotations=ToolAnnotations(destructiveHint=True))
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
        decay_result = await query_apply_decay(ctx, cutoff.isoformat() + "Z")
        result.decayed = len(decay_result) if decay_result else 0

    if find_similar:
        await ctx.info("Finding similar entities")
        entities = await query_all_entities_with_embedding(ctx)

        seen_pairs: set[tuple[str, str]] = set()
        deleted_ids: set[str] = set()

        for entity in entities:
            if str(entity['id']) in deleted_ids:
                continue

            similar = await query_similar_by_embedding(
                ctx, entity['embedding'], str(entity['id']), limit=10
            )

            for candidate in similar:
                if str(candidate['id']) in deleted_ids:
                    continue

                if candidate.get('sim', 0) >= similarity_threshold:
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
                            await query_delete_entity_by_record_id(ctx, delete_id)

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


@mcp.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
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
        entity_result = await query_entity_with_embedding(ctx, entity_id)
        entity = entity_result[0] if entity_result else None

        if entity:
            similar = await query_similar_for_contradiction(ctx, entity['embedding'], entity_id)

            for other in similar:
                nli_result = check_contradiction(entity['content'], other['content'])
                if nli_result['label'] == 'contradiction':
                    contradictions.append(Contradiction(
                        entity1=EntityResult(id=entity_id, content=entity['content']),
                        entity2=EntityResult(id=str(other['id']), content=other['content']),
                        confidence=nli_result['scores']['contradiction']
                    ))
    else:
        entities = await query_entities_by_labels(ctx, labels)

        for i, e1 in enumerate(entities):
            for e2 in entities[i+1:]:
                sim_result = await query_vector_similarity(ctx, e1['embedding'], e2['embedding'])
                sim = sim_result[0]['sim'] if sim_result else 0

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
    entity_count = await query_count_entities(ctx)
    total_entities = entity_count[0]['count'] if entity_count else 0

    relation_count = await query_count_relations(ctx)
    total_relations = relation_count[0]['count'] if relation_count and relation_count[0] else 0

    labels_result = await query_get_all_labels(ctx)
    all_labels = labels_result[0]['labels'] if labels_result else []

    # Count entities per label
    label_counts: dict[str, int] = {}
    for label in all_labels:
        count_result = await query_count_by_label(ctx, label)
        label_counts[label] = count_result[0]['count'] if count_result else 0

    return MemoryStats(
        total_entities=total_entities,
        total_relations=total_relations,
        labels=all_labels,
        label_counts=label_counts
    )


@mcp.resource("memory://labels")
async def get_all_labels_resource(ctx: Context) -> list[str]:
    """Get all labels/categories used in memory."""
    results = await query_get_all_labels(ctx)
    return results[0]['labels'] if results else []


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
