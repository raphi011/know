"""Episode tools: add_episode, search_episodes, get_episode, delete_episode."""

import logging
import time
from datetime import datetime

from fastmcp import FastMCP, Context

logger = logging.getLogger("memcp.episode")
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import EntityResult, EpisodeResult, EpisodeSearchResult
from memcp.utils import embed, log_op, extract_record_id
from memcp.db import (
    detect_context,
    query_create_episode,
    query_search_episodes,
    query_get_episode,
    query_update_episode_access,
    query_link_entity_to_episode,
    query_get_episode_entities,
    query_delete_episode,
)

server = FastMCP("episode")


@server.tool(annotations=ToolAnnotations(destructiveHint=False))
async def add_episode(
    content: str,
    timestamp: str | None = None,
    summary: str | None = None,
    metadata: dict | None = None,
    context: str | None = None,
    entity_ids: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> EpisodeResult:
    """Store a complete interaction episode as first-class memory.

    Use when storing full conversation sessions, meeting notes, or significant
    interactions that should be recalled as a unit.

    Args:
        content: Full episode content (conversation, notes, etc.)
        timestamp: When episode occurred (ISO format, defaults to now)
        summary: Optional brief summary
        metadata: Flexible metadata dict (session_id, source, participants, etc.)
        context: Project namespace (uses default if not provided)
        entity_ids: Entity IDs to link as extracted from this episode
    """
    start = time.time()

    if not content or not content.strip():
        raise ToolError("Episode content cannot be empty")

    # Generate episode ID from timestamp
    ts = timestamp or datetime.now().isoformat()
    episode_id = f"ep_{ts.replace(':', '-').replace('.', '-').replace('+', '-')}"

    # Detect context
    effective_context = detect_context(context)

    await ctx.info(f"Storing episode: {episode_id}")

    # Generate embedding (truncate for embedding model if needed)
    embedding = embed(content[:8000])

    await query_create_episode(
        ctx,
        episode_id=episode_id,
        content=content,
        embedding=embedding,
        timestamp=ts,
        summary=summary,
        metadata=metadata or {},
        context=effective_context
    )

    # Link entities if provided
    linked_count = 0
    if entity_ids:
        for i, entity_id in enumerate(entity_ids):
            try:
                await query_link_entity_to_episode(
                    ctx, entity_id, episode_id, position=i, confidence=1.0
                )
                linked_count += 1
            except Exception as e:
                logger.debug(f"Failed to link entity {entity_id} to episode: {e}")

    log_op('add_episode', start, episode_id=episode_id, content_len=len(content))

    return EpisodeResult(
        id=f"episode:{episode_id}",
        content=content[:500] + "..." if len(content) > 500 else content,
        timestamp=ts,
        summary=summary,
        metadata=metadata or {},
        context=effective_context,
        linked_entities=linked_count
    )


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search_episodes(
    query: str,
    time_start: str | None = None,
    time_end: str | None = None,
    context: str | None = None,
    limit: int = 10,
    ctx: Context = None  # type: ignore[assignment]
) -> EpisodeSearchResult:
    """Search past interaction episodes by content and time range.

    Use for queries like "what did we discuss last week?" or
    "find conversations about project X".

    Args:
        query: Semantic search query
        time_start: Filter episodes after this time (ISO format)
        time_end: Filter episodes before this time (ISO format)
        context: Filter by project namespace
        limit: Max results (1-50)
    """
    start = time.time()

    if not query or not query.strip():
        raise ToolError("Query cannot be empty")
    if limit < 1 or limit > 50:
        raise ToolError("Limit must be between 1 and 50")

    await ctx.info(f"Searching episodes: {query[:50]}...")

    query_embedding = embed(query)

    episodes = await query_search_episodes(
        ctx, query, query_embedding, time_start, time_end, context, limit
    )

    # Track access for each found episode
    for ep in episodes:
        try:
            await query_update_episode_access(ctx, extract_record_id(ep['id']))
        except Exception as e:
            logger.debug(f"Failed to update episode access: {e}")

    results = [EpisodeResult(
        id=str(e['id']),
        content=e.get('content', '')[:500] + "..." if len(e.get('content', '')) > 500 else e.get('content', ''),
        timestamp=str(e.get('timestamp', '')),
        summary=e.get('summary'),
        metadata=e.get('metadata') or {},
        context=e.get('context')
    ) for e in episodes]

    log_op('search_episodes', start, query=query[:30], results=len(results))
    return EpisodeSearchResult(episodes=results, count=len(results))


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def get_episode(
    episode_id: str,
    include_entities: bool = False,
    ctx: Context = None  # type: ignore[assignment]
) -> EpisodeResult | None:
    """Retrieve a specific episode by ID, optionally with extracted entities.

    Args:
        episode_id: The episode ID (with or without 'episode:' prefix)
        include_entities: If True, include full data of linked entities
    """
    if not episode_id or not episode_id.strip():
        raise ToolError("Episode ID cannot be empty")

    # Strip "episode:" prefix if present
    clean_id = episode_id.replace("episode:", "")

    results = await query_get_episode(ctx, clean_id)
    episode = results[0] if results else None

    if episode:
        # Track access
        try:
            await query_update_episode_access(ctx, clean_id)
        except Exception as e:
            logger.debug(f"Failed to update episode access for {clean_id}: {e}")

        entities_data: list[EntityResult] | None = None
        linked_count = 0

        if include_entities:
            entity_results = await query_get_episode_entities(ctx, clean_id)
            if entity_results and entity_results[0].get('entities'):
                entities_list = entity_results[0]['entities']
                linked_count = len(entities_list)
                entities_data = [EntityResult(
                    id=str(e.get('id', '')),
                    type=e.get('type'),
                    labels=e.get('labels', []),
                    content=e.get('content', ''),
                    confidence=e.get('confidence'),
                    source=e.get('source'),
                    context=e.get('context'),
                    importance=e.get('importance')
                ) for e in entities_list if isinstance(e, dict)]

        return EpisodeResult(
            id=str(episode['id']),
            content=episode.get('content', ''),
            timestamp=str(episode.get('timestamp', '')),
            summary=episode.get('summary'),
            metadata=episode.get('metadata') or {},
            context=episode.get('context'),
            linked_entities=linked_count,
            entities=entities_data
        )
    return None


@server.tool(annotations=ToolAnnotations(destructiveHint=True))
async def delete_episode(episode_id: str, ctx: Context) -> str:
    """Delete an episode from memory.

    Args:
        episode_id: The episode ID to delete (with or without 'episode:' prefix)
    """
    start = time.time()
    if not episode_id or not episode_id.strip():
        raise ToolError("Episode ID cannot be empty")

    # Strip "episode:" prefix if present
    clean_id = episode_id.replace("episode:", "")

    await ctx.info(f"Deleting episode: {clean_id}")
    await query_delete_episode(ctx, clean_id)
    log_op('delete_episode', start, episode_id=clean_id)
    return f"Removed episode:{clean_id}"
