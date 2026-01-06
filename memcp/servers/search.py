"""Search-related tools: search, get_entity, list_labels."""

import time

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import EntityResult, SearchResult, ContextStats, ContextListResult, EntityTypeInfo, EntityTypeListResult
from memcp.utils import embed, log_op, extract_record_id
from memcp.db import (
    detect_context,
    query_hybrid_search, query_update_access, query_get_entity, query_list_labels,
    query_list_contexts, query_get_context_stats,
    query_list_types, query_entities_by_type,
    get_entity_types, ALLOW_CUSTOM_TYPES
)

server = FastMCP("search")


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    context: str | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> SearchResult:
    """Search your persistent memory for previously stored knowledge. Use when the user asks 'do you remember...', 'what do you know about...', 'recall...', or needs context from past conversations. Combines semantic similarity and keyword matching.

    Args:
        query: The search query
        labels: Optional list of labels to filter by
        limit: Max results (1-100)
        context: Optional project namespace to filter by
    """
    start = time.time()

    if not query or not query.strip():
        raise ToolError("Query cannot be empty")
    if limit < 1 or limit > 100:
        raise ToolError("Limit must be between 1 and 100")

    await ctx.info(f"Searching for: {query[:50]}...")

    query_embedding = embed(query)
    filter_labels = labels or []
    effective_context = detect_context(context)

    # Hybrid search: BM25 keyword matching + vector similarity (RRF fusion)
    entities = await query_hybrid_search(
        ctx, query, query_embedding, filter_labels, limit, effective_context
    )

    # Handle None result from RRF search
    if entities is None:
        entities = []

    # Track access patterns
    for r in entities:
        await query_update_access(ctx, extract_record_id(r['id']))

    await ctx.info(f"Found {len(entities)} results")

    entity_results = [EntityResult(
        id=str(e['id']),
        type=e.get('type'),
        labels=e.get('labels', []),
        content=e['content'],
        confidence=e.get('confidence'),
        source=e.get('source'),
        decay_weight=e.get('decay_weight'),
        context=e.get('context'),
        importance=e.get('importance')
    ) for e in entities]

    log_op('search', start, query=query[:30], limit=limit, results=len(entity_results))
    return SearchResult(entities=entity_results, count=len(entity_results), summary=None)


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
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
            decay_weight=entity.get('decay_weight'),
            context=entity.get('context'),
            importance=entity.get('importance')
        )
    return None


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def list_labels(ctx: Context) -> list[str]:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    results = await query_list_labels(ctx)
    return results[0]['labels'] if results else []


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def list_contexts(ctx: Context) -> ContextListResult:
    """List all project contexts/namespaces in memory. Use to see what projects have stored memories."""
    results = await query_list_contexts(ctx)
    contexts = results[0].get('contexts', []) if results else []
    return ContextListResult(contexts=contexts, count=len(contexts))


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def get_context_stats(context: str, ctx: Context) -> ContextStats:
    """Get statistics for a specific project context/namespace.

    Args:
        context: The context name to get stats for
    """
    if not context or not context.strip():
        raise ToolError("Context cannot be empty")

    results = await query_get_context_stats(ctx, context)
    stats = results[0] if results else {}

    return ContextStats(
        context=context,
        entities=stats.get('entities', 0),
        episodes=stats.get('episodes', 0),
        relations=stats.get('relations', 0)
    )


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def list_entity_types(
    context: str | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> EntityTypeListResult:
    """List all available entity types with their descriptions and counts.

    Returns both predefined types (preference, requirement, procedure, etc.)
    and any custom types in use. Use to understand what kinds of entities
    are stored or when categorizing new knowledge.

    Args:
        context: Optional context to filter type counts by
    """
    # Get predefined types with descriptions
    predefined = get_entity_types()

    # Get types actually in use with counts
    used_types = await query_list_types(ctx, context)
    type_counts = {t['type']: t.get('count', 0) for t in used_types if t.get('type')}

    # Build result: predefined types + any custom types found
    result_types = []

    # Add predefined types (even if count is 0)
    for type_name, description in sorted(predefined.items()):
        result_types.append(EntityTypeInfo(
            type=type_name,
            description=description,
            count=type_counts.get(type_name, 0)
        ))

    # Add any custom types not in predefined list
    for type_name, count in sorted(type_counts.items()):
        if type_name not in predefined:
            result_types.append(EntityTypeInfo(
                type=type_name,
                description="(custom type)",
                count=count
            ))

    return EntityTypeListResult(
        types=result_types,
        custom_types_allowed=ALLOW_CUSTOM_TYPES
    )


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search_by_type(
    entity_type: str,
    context: str | None = None,
    limit: int = 50,
    ctx: Context = None  # type: ignore[assignment]
) -> SearchResult:
    """Search entities by type (e.g., 'preference', 'requirement', 'procedure').

    Use for queries like "show all my preferences" or "list all decisions".

    Args:
        entity_type: Entity type to filter by
        context: Optional project namespace to filter by
        limit: Max results (1-100)
    """
    if not entity_type or not entity_type.strip():
        raise ToolError("Entity type cannot be empty")
    if limit < 1 or limit > 100:
        raise ToolError("Limit must be between 1 and 100")

    effective_context = detect_context(context)

    entities = await query_entities_by_type(ctx, entity_type.lower(), effective_context, limit)

    entity_results = [EntityResult(
        id=str(e['id']),
        type=e.get('type'),
        labels=e.get('labels', []),
        content=e['content'],
        confidence=e.get('confidence'),
        source=e.get('source'),
        decay_weight=e.get('decay_weight'),
        context=e.get('context'),
        importance=e.get('importance')
    ) for e in entities]

    return SearchResult(entities=entity_results, count=len(entity_results), summary=None)
