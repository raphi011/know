"""Persistence tools: remember, forget."""

import logging
import time

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import EntityResult, RememberResult, Contradiction
from memcp.utils import embed, check_contradiction, log_op
from memcp.db import (
    validate_entity, validate_relation,
    query_upsert_entity, query_create_relation, query_delete_entity,
    query_similar_for_contradiction
)

logger = logging.getLogger("memcp")
server = FastMCP("persist")


@server.tool(annotations=ToolAnnotations(destructiveHint=False))
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
    start = time.time()
    entities = entities or []
    relations = relations or []

    # Validate all inputs before any DB operations
    for entity in entities:
        validate_entity(entity)

    for relation in relations:
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

    log_op('remember', start, entities=len(entities), relations=len(relations),
           contradictions=len(result.contradictions))
    return result


@server.tool(annotations=ToolAnnotations(destructiveHint=True))
async def forget(entity_id: str, ctx: Context) -> str:
    """Delete information from persistent memory. Use when the user says 'forget this', 'remove...', 'delete...', or when information is explicitly outdated or wrong."""
    start = time.time()
    if not entity_id or not entity_id.strip():
        raise ToolError("ID cannot be empty")

    await ctx.info(f"Deleting entity: {entity_id}")
    await query_delete_entity(ctx, entity_id)
    log_op('forget', start, entity_id=entity_id)
    return f"Removed {entity_id}"
