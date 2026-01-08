# MCP SurrealDB Server

An MCP (Model Context Protocol) server in Python that connects to a SurrealDB instance to persist knowledge between agent sessions.

## Purpose

This server enables AI agents to store and retrieve knowledge across sessions, providing a persistent memory layer using SurrealDB as the backend database.

## Tech Stack

- **Language**: Python
- **Protocol**: MCP (Model Context Protocol)
- **Database**: SurrealDB

## Features (Planned)

- Connect to a SurrealDB instance
- Store knowledge/memories from agent sessions
- Retrieve relevant knowledge for new sessions
- Query and search stored information
- Manage knowledge lifecycle (create, read, update, delete)

## Development Workflow

**IMPORTANT**: Before committing any changes, always run all tests:

```bash
# Unit test (fast, ensures module compiles)
uv run pytest test_memcp.py -v

# Integration tests (requires SurrealDB running)
uv run pytest test_integration.py -v
```

The unit test ensures the module compiles correctly. Integration tests verify the MCP tools and database queries work properly. Both must pass before committing.

## SurrealDB Reference

For SurrealDB-specific syntax, v3.0 breaking changes, and query patterns:
- **Subagent**: Use the `surrealdb` subagent for complex query work (has built-in reference guide)

## TODO

### MCP Sampling Support

Claude Code does not currently support MCP sampling (see [Issue #1785](https://github.com/anthropics/claude-code/issues/1785)). Features removed due to this limitation:

- `memorize_file` tool - was using LLM to extract entities from documents
- `auto_tag` parameter in `remember` - was using LLM to generate labels
- `summarize` parameter in `search` - was using LLM to summarize results

**When Claude Code adds sampling support**, re-implement these features. The original implementation used:
```python
async def sample(ctx: Context, prompt: str, max_tokens: int = 1000) -> str:
    result = await ctx.request_context.session.create_message(
        messages=[types.SamplingMessage(role="user", content=types.TextContent(type="text", text=prompt))],
        max_tokens=max_tokens
    )
    return result.content.text
```

**Alternative approach**: Call Anthropic API directly with an API key instead of relying on MCP sampling.

### Docling Integration

Add [docling](https://github.com/docling-project/docling) for document parsing. Would enable `memorize_file` to handle PDFs, DOCX, PPTX, images, scanned docs, etc.

- Pure Python library, no deployment needed
- Converts documents → markdown/text → extract entities → store in SurrealDB
- Install: `uv add docling`

## Documentation

**IMPORTANT**: When adding or modifying features, always update `README.md` with example prompts showcasing what the feature can do. This helps users understand how to use each tool effectively.
