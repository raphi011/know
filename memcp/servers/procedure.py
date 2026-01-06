"""Procedure tools: add_procedure, get_procedure, search_procedures, list_procedures, delete_procedure."""

import logging
import time
from typing import Any

from fastmcp import FastMCP, Context

logger = logging.getLogger("memcp.procedure")
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.models import ProcedureResult, ProcedureStep, ProcedureSearchResult
from memcp.utils import embed, log_op, extract_record_id
from memcp.db import (
    detect_context, get_db,
    query_create_procedure,
    query_get_procedure,
    query_search_procedures,
    query_update_procedure_access,
    query_delete_procedure,
    query_list_procedures,
)

server = FastMCP("procedure")


def parse_steps(steps: list[dict[str, Any]]) -> list[ProcedureStep]:
    """Parse step dicts into ProcedureStep models."""
    result = []
    for i, step in enumerate(steps):
        if isinstance(step, dict):
            result.append(ProcedureStep(
                order=step.get('order', i + 1),
                content=step.get('content', str(step)),
                optional=step.get('optional', False)
            ))
        elif isinstance(step, str):
            result.append(ProcedureStep(
                order=i + 1,
                content=step,
                optional=False
            ))
    return result


def steps_to_dicts(steps: list[ProcedureStep]) -> list[dict[str, Any]]:
    """Convert ProcedureStep models to dicts for storage."""
    return [
        {'order': s.order, 'content': s.content, 'optional': s.optional}
        for s in steps
    ]


@server.tool(annotations=ToolAnnotations(destructiveHint=False))
async def add_procedure(
    name: str,
    description: str,
    steps: list[dict[str, Any]],
    context: str | None = None,
    labels: list[str] | None = None,
    ctx: Context = None  # type: ignore[assignment]
) -> ProcedureResult:
    """Store a step-by-step procedure or workflow.

    Use for capturing learned processes like deployment steps, debugging workflows,
    or any multi-step task that should be remembered for future reference.

    Args:
        name: Short name for the procedure (e.g., "Deploy to production")
        description: Brief description of what this procedure does
        steps: List of steps, each with {order, content, optional}
               - order: step number (auto-assigned if not provided)
               - content: step description
               - optional: whether step can be skipped (default: false)
        context: Project namespace (uses default if not provided)
        labels: Tags for categorization (e.g., ["deployment", "devops"])

    Example:
        add_procedure(
            name="Deploy app",
            description="Deploy the application to production",
            steps=[
                {"content": "Run tests", "optional": false},
                {"content": "Build Docker image"},
                {"content": "Push to registry"},
                {"content": "Update Kubernetes deployment"}
            ],
            labels=["deployment"]
        )
    """
    start = time.time()

    if not name or not name.strip():
        raise ToolError("Procedure name cannot be empty")
    if not description or not description.strip():
        raise ToolError("Procedure description cannot be empty")
    if not steps or len(steps) == 0:
        raise ToolError("Procedure must have at least one step")

    # Generate procedure ID from name
    procedure_id = name.lower().replace(' ', '-').replace('_', '-')
    # Remove any chars that aren't alphanumeric or dash
    procedure_id = ''.join(c for c in procedure_id if c.isalnum() or c == '-')

    # Detect context
    effective_context = detect_context(context)

    await ctx.info(f"Storing procedure: {name}")
    db = get_db(ctx)

    # Parse and validate steps
    parsed_steps = parse_steps(steps)
    steps_dicts = steps_to_dicts(parsed_steps)

    # Generate embedding from name + description + step contents
    embed_text = f"{name}. {description}. " + " ".join(s.content for s in parsed_steps)
    embedding = embed(embed_text[:8000])

    await query_create_procedure(
        db,
        procedure_id=procedure_id,
        name=name,
        description=description,
        steps=steps_dicts,
        embedding=embedding,
        context=effective_context,
        labels=labels or []
    )

    log_op('add_procedure', start, procedure_id=procedure_id, steps=len(parsed_steps))

    return ProcedureResult(
        id=f"procedure:{procedure_id}",
        name=name,
        description=description,
        steps=parsed_steps,
        context=effective_context,
        labels=labels or []
    )


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def get_procedure(
    procedure_id: str,
    ctx: Context = None  # type: ignore[assignment]
) -> ProcedureResult | None:
    """Retrieve a specific procedure by ID.

    Args:
        procedure_id: The procedure ID (with or without 'procedure:' prefix)
    """
    if not procedure_id or not procedure_id.strip():
        raise ToolError("Procedure ID cannot be empty")

    # Strip "procedure:" prefix if present
    clean_id = procedure_id.replace("procedure:", "")
    db = get_db(ctx)

    results = await query_get_procedure(db, clean_id)
    proc = results[0] if results else None

    if proc:
        # Track access
        try:
            await query_update_procedure_access(db, clean_id)
        except Exception as e:
            logger.debug(f"Failed to update procedure access for {clean_id}: {e}")

        steps = parse_steps(proc.get('steps', []))

        return ProcedureResult(
            id=str(proc['id']),
            name=proc.get('name', ''),
            description=proc.get('description', ''),
            steps=steps,
            context=proc.get('context'),
            labels=proc.get('labels', [])
        )
    return None


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def search_procedures(
    query: str,
    context: str | None = None,
    labels: list[str] | None = None,
    limit: int = 10,
    ctx: Context = None  # type: ignore[assignment]
) -> ProcedureSearchResult:
    """Search for procedures by name, description, or step content.

    Use for queries like "how do I deploy?" or "show me testing workflows".

    Args:
        query: Semantic search query
        context: Filter by project namespace
        labels: Filter by tags
        limit: Max results (1-50)
    """
    start = time.time()

    if not query or not query.strip():
        raise ToolError("Query cannot be empty")
    if limit < 1 or limit > 50:
        raise ToolError("Limit must be between 1 and 50")

    await ctx.info(f"Searching procedures: {query[:50]}...")
    db = get_db(ctx)

    query_embedding = embed(query)

    procedures = await query_search_procedures(
        db, query, query_embedding, context, labels or [], limit
    )

    # Handle None result from RRF search
    if procedures is None:
        procedures = []

    # Track access for each found procedure
    for proc in procedures:
        try:
            await query_update_procedure_access(db, extract_record_id(proc['id']))
        except Exception as e:
            logger.debug(f"Failed to update procedure access: {e}")

    results = [ProcedureResult(
        id=str(p['id']),
        name=p.get('name', ''),
        description=p.get('description', ''),
        steps=parse_steps(p.get('steps', [])),
        context=p.get('context'),
        labels=p.get('labels', [])
    ) for p in procedures]

    log_op('search_procedures', start, query=query[:30], results=len(results))
    return ProcedureSearchResult(procedures=results, count=len(results))


@server.tool(annotations=ToolAnnotations(readOnlyHint=True))
async def list_procedures(
    context: str | None = None,
    limit: int = 50,
    ctx: Context = None  # type: ignore[assignment]
) -> ProcedureSearchResult:
    """List all stored procedures, optionally filtered by context.

    Args:
        context: Filter by project namespace
        limit: Max results (1-100)
    """
    if limit < 1 or limit > 100:
        raise ToolError("Limit must be between 1 and 100")
    db = get_db(ctx)

    procedures = await query_list_procedures(db, context, limit)

    results = [ProcedureResult(
        id=str(p['id']),
        name=p.get('name', ''),
        description=p.get('description', ''),
        steps=[],  # list_procedures returns summary only (step_count)
        context=p.get('context'),
        labels=p.get('labels', [])
    ) for p in procedures]

    return ProcedureSearchResult(procedures=results, count=len(results))


@server.tool(annotations=ToolAnnotations(destructiveHint=True))
async def delete_procedure(
    procedure_id: str,
    ctx: Context = None  # type: ignore[assignment]
) -> str:
    """Delete a procedure from memory.

    Args:
        procedure_id: The procedure ID to delete (with or without 'procedure:' prefix)
    """
    start = time.time()
    if not procedure_id or not procedure_id.strip():
        raise ToolError("Procedure ID cannot be empty")

    # Strip "procedure:" prefix if present
    clean_id = procedure_id.replace("procedure:", "")

    await ctx.info(f"Deleting procedure: {clean_id}")
    db = get_db(ctx)
    await query_delete_procedure(db, clean_id)
    log_op('delete_procedure', start, procedure_id=clean_id)
    return f"Removed procedure:{clean_id}"
