"""Search-related tools: search, get_entity, list_labels."""

import time

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import EntityResult, SearchResult
from memcp.utils import embed, log_op
from memcp.db import (
    query_hybrid_search, query_update_access, query_get_entity, query_list_labels
)

server = FastMCP("search")


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search(
    query: str,
    labels: list[str] | None = None,
    limit: int = 10,
    ctx: Context = None  # type: ignore[assignment]
) -> SearchResult:
    """Search your persistent memory for previously stored knowledge. Use when the user asks 'do you remember...', 'what do you know about...', 'recall...', or needs context from past conversations. Combines semantic similarity and keyword matching."""
    start = time.time()

    if not query or not query.strip():
        raise ToolError("Query cannot be empty")
    if limit < 1 or limit > 100:
        raise ToolError("Limit must be between 1 and 100")

    await ctx.info(f"Searching for: {query[:50]}...")

    query_embedding = embed(query)
    filter_labels = labels or []

    # Hybrid search: BM25 keyword matching + vector similarity (RRF fusion)
    entities = await query_hybrid_search(
        ctx, query, query_embedding, filter_labels, limit
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
            decay_weight=entity.get('decay_weight')
        )
    return None


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def list_labels(ctx: Context) -> list[str]:
    """List all categories/tags used to organize memories. Use to understand what topics are stored or when the user asks 'what do you remember about' without a specific topic."""
    results = await query_list_labels(ctx)
    return results[0]['labels'] if results else []
