"""MCP SurrealDB Server - Persistent memory for AI agents."""

__version__ = "0.1.6"


def main():
    """Entry point - lazy import to avoid loading models on package import."""
    from memcp.server import main as _main
    return _main()


__all__ = ['main']
