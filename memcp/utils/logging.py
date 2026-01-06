"""Structured logging helpers for memcp."""

import logging
import time

logger = logging.getLogger("memcp")


def log_op(operation: str, start_time: float, **details) -> None:
    """Log operation with timing and details."""
    duration_ms = (time.time() - start_time) * 1000
    detail_str = ' '.join(f'{k}={v}' for k, v in details.items())
    logger.info(f"[{operation}] {duration_ms:.1f}ms {detail_str}")
