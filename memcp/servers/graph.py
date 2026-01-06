"""Graph traversal tools: traverse, find_path."""

import json

from fastmcp import FastMCP, Context
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations

from memcp.db import get_db, query_traverse, query_find_path

server = FastMCP("graph")


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
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
    db = get_db(ctx)

    results = await query_traverse(db, start, depth, relation_types or None)
    return json.dumps(results[0] if results else [], indent=2, default=str)


@server.tool(annotations=ToolAnnotations(readOnlyHint=True, idempotentHint=True))
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
    db = get_db(ctx)

    results = await query_find_path(db, from_id, to_id, max_depth)
    return json.dumps(results[0] if results else [], indent=2, default=str)
