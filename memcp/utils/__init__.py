"""Utility functions for memcp."""

from typing import Any

from memcp.utils.embedding import embed, check_contradiction, get_embed_cache_stats
from memcp.utils.logging import log_op


def extract_record_id(record_id: str | Any) -> str:
    """Extract entity ID from full record ID.

    SurrealDB returns IDs like 'entity:user123' or 'procedure:deploy'.
    This extracts just the ID part ('user123', 'deploy').

    Args:
        record_id: Full record ID or just the ID part

    Returns:
        The ID part without table prefix
    """
    if record_id is None:
        return ""
    record_str = str(record_id)
    return record_str.split(':', 1)[1] if ':' in record_str else record_str


__all__ = ['embed', 'check_contradiction', 'get_embed_cache_stats', 'log_op', 'extract_record_id']
