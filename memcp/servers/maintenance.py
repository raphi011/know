"""Maintenance tools: reflect, check_contradictions."""

import time
from datetime import datetime, timedelta

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import (
    EntityResult, SimilarPair, Contradiction, ReflectResult
)
from memcp.utils import check_contradiction, log_op
from memcp.db import (
    query_apply_decay, query_all_entities_with_embedding, query_similar_by_embedding,
    query_delete_entity_by_record_id, query_entity_with_embedding,
    query_similar_for_contradiction, query_entities_by_labels, query_vector_similarity,
    query_batch_recalculate_importance
)

server = FastMCP("maintenance")


@server.tool(annotations=ToolAnnotations(destructiveHint=True))
async def reflect(
    apply_decay: bool = True,
    decay_days: int = 30,
    find_similar: bool = True,
    similarity_threshold: float = 0.85,
    auto_merge: bool = False,
    recalculate_importance: bool = True,
    context: str | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> ReflectResult:
    """Maintenance tool to clean up memory: decay old unused knowledge, find duplicates, identify clusters. Use periodically or when the user asks to 'clean up', 'organize', or 'consolidate' memories.

    When auto_merge is True, duplicate entities (similarity >= threshold) are automatically
    merged: the entity with higher access_count is kept, the other is deleted. If access
    counts are equal, the more recently accessed entity is kept.

    Args:
        apply_decay: Apply temporal decay to old memories
        decay_days: Days of inactivity before decay applies
        find_similar: Find similar/duplicate entities
        similarity_threshold: Threshold for similarity detection (0-1)
        auto_merge: Automatically merge duplicates
        recalculate_importance: Recalculate importance scores for all entities
        context: Optional context to scope the operation
    """
    start = time.time()
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

    if recalculate_importance:
        await ctx.info("Recalculating importance scores")
        recalc_count = await query_batch_recalculate_importance(ctx, context)
        result.importance_recalculated = recalc_count

    await ctx.info(f"Reflect complete: {result.decayed} decayed, {len(result.similar_pairs)} similar, {result.merged} merged, {result.importance_recalculated} importance recalculated")
    log_op('reflect', start, decayed=result.decayed, similar=len(result.similar_pairs), merged=result.merged, importance_recalc=result.importance_recalculated)
    return result


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
async def check_contradictions_tool(
    entity_id: str | None = None,
    labels: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> list[Contradiction]:
    """Detect conflicting information in memory. Use when the user asks to verify consistency, or proactively before storing facts that might conflict with existing knowledge."""
    start = time.time()
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
    log_op('check_contradictions', start, entity_id=entity_id, labels=len(labels), found=len(contradictions))
    return contradictions
