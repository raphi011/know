"""Main memcp server - mounts sub-servers and defines resources/prompts."""

import logging
import os
import sys

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

# Import sub-servers
from memcp.servers import search_server, graph_server, persist_server, maintenance_server, episode_server, procedure_server

# Import db module for lifespan and query functions
from memcp.db import (
    app_lifespan,
    query_count_entities, query_count_relations,
    query_get_all_labels, query_count_by_label,
    query_list_contexts, query_count_episodes
)
from memcp.models import MemoryStats, ContextListResult

# Start loading models in background (non-blocking)
from memcp.utils.embedding import preload_models
preload_models()

# Create main server with lifespan
mcp = FastMCP("memcp", lifespan=app_lifespan)

# Mount sub-servers (no prefix - tools keep original names)
mcp.mount(search_server, prefix="")
mcp.mount(graph_server, prefix="")
mcp.mount(persist_server, prefix="")
mcp.mount(maintenance_server, prefix="")
mcp.mount(episode_server, prefix="")
mcp.mount(procedure_server, prefix="")


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


@mcp.resource("memory://contexts")
async def get_all_contexts_resource(ctx: Context) -> ContextListResult:
    """Get all project contexts/namespaces in memory."""
    results = await query_list_contexts(ctx)
    contexts = results[0].get('contexts', []) if results else []
    return ContextListResult(contexts=contexts, count=len(contexts))


@mcp.resource("memory://episodes/count")
async def get_episode_count_resource(ctx: Context) -> int:
    """Get total episode count."""
    results = await query_count_episodes(ctx)
    return results[0].get('count', 0) if results else 0


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


@mcp.prompt()
def recall_session(time_description: str) -> str:
    """Generate a prompt to recall what happened in a past session."""
    return f"""Please search my episodic memory for sessions {time_description}.

Use search_episodes to find relevant sessions and summarize:
- What topics were discussed
- What decisions were made
- Any important outcomes or action items

Provide a chronological summary of the sessions found."""


@mcp.prompt()
def project_context(project_name: str) -> str:
    """Generate a prompt to get all context about a specific project."""
    return f"""Please gather all stored knowledge about the project "{project_name}".

1. Search entities with context="{project_name}"
2. Search episodes with context="{project_name}"
3. Look for any related memories that might be connected

Provide a comprehensive overview of what's known about this project."""


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
