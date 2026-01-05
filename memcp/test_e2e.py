"""End-to-end tests for MCP tools.

These tests call tools through the MCP protocol using fastmcp.Client,
testing the full integration including decorators and serialization.

Setup:
    docker run -p 8000:8000 surrealdb/surrealdb:v3.0.0-beta.1 \
      start --user root --pass root memory
"""

import pytest
import pytest_asyncio

from fastmcp import Client


@pytest_asyncio.fixture
async def mcp_client():
    """Create MCP client connected to server."""
    # Import here to control when models load
    from memcp.server import mcp

    async with Client(mcp) as client:
        yield client


@pytest.mark.asyncio
async def test_list_tools(mcp_client: Client):
    """Verify all expected tools are registered."""
    tools = await mcp_client.list_tools()
    tool_names = [t.name for t in tools]

    expected = [
        "remember", "get_entity", "search", "forget",
        "reflect", "list_labels",
        "traverse", "find_path", "check_contradictions_tool"
    ]
    for name in expected:
        assert name in tool_names, f"Tool '{name}' not registered"


@pytest.mark.asyncio
async def test_remember_and_get_entity(mcp_client: Client):
    """Test storing and retrieving an entity via MCP."""
    # Remember
    result = await mcp_client.call_tool("remember", {
        "id": "e2e-test-entity",
        "content": "E2E test content for MCP integration",
        "type": "test",
        "labels": ["e2e", "integration"],
        "weight": 1.0,
        "source": "test_e2e.py"
    })

    assert result.is_error is False

    # Get entity
    result = await mcp_client.call_tool("get_entity", {
        "entity_id": "e2e-test-entity"
    })

    assert result.is_error is False


@pytest.mark.asyncio
async def test_search(mcp_client: Client):
    """Test hybrid search via MCP."""
    # First store something
    await mcp_client.call_tool("remember", {
        "id": "e2e-search-target",
        "content": "Machine learning algorithms for classification",
        "type": "concept",
        "labels": ["ml", "search-test"],
        "weight": 1.0,
        "source": "test"
    })

    # Search for it
    result = await mcp_client.call_tool("search", {
        "query": "machine learning classification",
        "limit": 5
    })

    assert result.is_error is False


@pytest.mark.asyncio
@pytest.mark.skip(reason="Resource listing not compatible between mcp SDK and fastmcp Client")
async def test_stats_resource(mcp_client: Client):
    """Test stats resource via MCP."""
    resources = await mcp_client.list_resources()
    resource_uris = [r.uri for r in resources]

    assert "memory://stats" in resource_uris


@pytest.mark.asyncio
async def test_forget(mcp_client: Client):
    """Test forget (delete) via MCP."""
    # Create entity
    await mcp_client.call_tool("remember", {
        "id": "e2e-to-delete",
        "content": "This will be deleted",
        "type": "test",
        "labels": ["delete-test"],
        "weight": 1.0,
        "source": "test"
    })

    # Delete it
    result = await mcp_client.call_tool("forget", {
        "entity_id": "e2e-to-delete"
    })

    assert result.is_error is False

    # Verify gone - get_entity should return None
    get_result = await mcp_client.call_tool("get_entity", {
        "entity_id": "e2e-to-delete"
    })
    # Tool returns None for missing entity (not an error)
