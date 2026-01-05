"""Build verification test for memcp."""


def test_module_compiles():
    """Verify memcp module and submodules compile and can be imported."""
    import memcp
    import memcp.server
    import memcp.db
    import memcp.models

    assert memcp.main is not None
    assert memcp.server.mcp is not None
    assert memcp.db.run_query is not None
    assert memcp.models.EntityResult is not None
