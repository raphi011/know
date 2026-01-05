"""Build verification test for memcp."""


def test_module_compiles():
    """Verify memcp module compiles and can be imported."""
    import memcp
    assert memcp.main is not None
